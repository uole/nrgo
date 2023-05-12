package nrgo

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"git.nspix.com/golang/kos/pkg/log"
	"git.nspix.com/golang/kos/util/pool"
	"github.com/rs/xid"
	"github.com/uole/nrgo/pkg/multiplex"
	"github.com/uole/nrgo/pkg/multiplex/kcp"
	"github.com/uole/nrgo/pkg/multiplex/quic"
	"github.com/uole/nrgo/pkg/multiplex/tcp"
	"github.com/uole/nrgo/pkg/packet"
	"io"
	"math"
	"net"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

var (
	defaultTimeout = time.Second * 5
)

const (
	defaultBufferSize = 16 * 1024

	protoQUIC = "quic"
	protoKCP  = "kcp"
	protoTCP  = "tcp"
)

type Connection struct {
	id          string
	secretKey   []byte
	conn        multiplex.Session
	info        *NodeInfo
	dialer      net.Dialer
	concurrency int32
	sequence    uint16
	activeStamp int64
	closeFlag   int32
	mutex       sync.Mutex
	closeChan   chan struct{}
}

func (conn *Connection) ID() string {
	return conn.id
}

func (conn *Connection) pipe(dst, src io.ReadWriteCloser, ch chan<- error) {
	var (
		err error
	)
	buf := pool.GetBytes(defaultBufferSize)
	defer pool.PutBytes(buf)
	_, err = io.CopyBuffer(dst, src, buf)
	select {
	case ch <- err:
	}
	return
}

func (conn *Connection) nextSequence() uint16 {
	conn.mutex.Lock()
	defer conn.mutex.Unlock()
	conn.sequence++
	if conn.sequence >= math.MaxUint16 {
		conn.sequence = 0
	}
	return conn.sequence
}

func (conn *Connection) handshake(request *packet.Frame, stream multiplex.Stream) (rwc io.ReadWriteCloser, err error) {
	var (
		timeout time.Duration
	)
	req := &packet.HandshakeRequest{}
	if err = json.Unmarshal(request.Buf, req); err != nil {
		return
	}
	timeout = req.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	res := &packet.HandshakeResponse{
		Host: req.Host,
	}
	if rwc, err = net.DialTimeout("tcp", req.Host, timeout); err == nil {
		res.Success = true
		err = packet.WriteFrame(stream, packet.NewFrame(packet.TypeHandshakeResponse, request.Sequence, res))
	} else {
		res.Reason = err.Error()
		_ = packet.WriteFrame(stream, packet.NewFrame(packet.TypeHandshakeResponse, request.Sequence, res))
	}
	return
}

func (conn *Connection) handlePing(request *packet.Frame, stream multiplex.Stream) {
	var (
		err error
	)
	if err = packet.WriteFrame(
		stream,
		packet.NewFrame(
			packet.TypeHandshakePong,
			request.Sequence,
			&packet.PongResponse{Timestamp: time.Now().Unix()},
		),
	); err != nil {
		log.Debugf("connection %s write pong error: %s", conn.ID(), err.Error())
	}
}

func (conn *Connection) handleTraffic(request *packet.Frame, stream multiplex.Stream) {
	var (
		err    error
		remote io.ReadWriteCloser
	)
	if remote, err = conn.handshake(request, stream); err != nil {
		log.Debugf("connection %s handshake error: %s", conn.ID(), err.Error())
		return
	}
	defer func() {
		err = stream.Close()
	}()
	errChan := make(chan error, 2)
	go conn.pipe(remote, stream, errChan)
	go conn.pipe(stream, remote, errChan)
	select {
	case <-conn.closeChan:
		err = io.ErrClosedPipe
	case err = <-errChan:
	}
}

func (conn *Connection) process(stream multiplex.Stream) {
	var (
		err   error
		frame *packet.Frame
	)
	atomic.StoreInt64(&conn.activeStamp, time.Now().Unix())
	atomic.AddInt32(&conn.concurrency, 1)
	defer func() {
		err = stream.Close()
		atomic.AddInt32(&conn.concurrency, -1)
	}()
	if frame, err = packet.ReadFrame(stream); err != nil {
		return
	}
	switch frame.Type {
	case packet.TypeHandshakePing:
		conn.handlePing(frame, stream)
	case packet.TypeHandshakeRequest:
		conn.handleTraffic(frame, stream)
	default:
		log.Debugf("connection %s receive unsupported request: %0x", conn.ID(), frame.Type)
	}
	return
}

func (conn *Connection) Dial(ctx context.Context, proto, addr string) (err error) {
	var (
		buf   []byte
		frame *packet.Frame
	)
	switch proto {
	case protoQUIC:
		conn.conn, err = quic.Dial(ctx, addr)
	case protoKCP:
		conn.conn, err = kcp.Dial(ctx, addr, func(opts *kcp.Options) {
			opts.Key = conn.secretKey
		})
	default:
		conn.conn, err = tcp.Dial(ctx, addr, func(opts *tcp.Options) {
			opts.Key = conn.secretKey
		})
	}
	if err != nil {
		return
	}
	req := &packet.RegisterRequest{
		ID:      conn.info.ID,
		OS:      runtime.GOOS,
		Name:    conn.info.Name,
		Country: conn.info.Country,
		IP:      conn.info.IP,
		CPU:     conn.info.CPU,
		Uptime:  conn.info.Uptime,
	}
	if err = conn.conn.WriteMessage(packet.NewFrame(packet.TypeRegisterRequest, conn.nextSequence(), req).Bytes()); err != nil {
		return
	}
	if buf, err = conn.conn.ReadMessage(); err != nil {
		return
	}
	if frame, err = packet.ReadFrame(bytes.NewReader(buf)); err != nil {
		return
	}
	res := &packet.RegisterResponse{}
	if err = json.Unmarshal(frame.Buf, res); err != nil {
		return
	}
	if !res.Success {
		err = errors.New(res.Reason)
	}
	return
}

func (conn *Connection) Close() (err error) {
	if !atomic.CompareAndSwapInt32(&conn.closeFlag, 0, 1) {
		return
	}
	close(conn.closeChan)
	err = conn.conn.Close()
	return
}

func (conn *Connection) IoLoop(ctx context.Context) error {
	for {
		if stream, err := conn.conn.AcceptStream(ctx); err != nil {
			return err
		} else {
			go conn.process(stream)
		}
	}
}

func NewConnection(secretKey []byte, info *NodeInfo) *Connection {
	return &Connection{
		id:        xid.New().String(),
		info:      info,
		secretKey: secretKey,
		closeChan: make(chan struct{}),
	}
}
