package nrgo

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"git.nspix.com/golang/kos/pkg/log"
	"git.nspix.com/golang/kos/util/pool"
	"github.com/rs/xid"
	goroutinePool "github.com/sourcegraph/conc/pool"
	"github.com/uole/nrgo/pkg/multiplex"
	"github.com/uole/nrgo/pkg/multiplex/kcp"
	"github.com/uole/nrgo/pkg/multiplex/quic"
	"github.com/uole/nrgo/pkg/multiplex/tcp"
	"github.com/uole/nrgo/pkg/packet"
	"io"
	"math"
	"net"
	"runtime"
	"runtime/debug"
	"sync/atomic"
	"time"
)

var (
	defaultTimeout = time.Second * 5
)

const (
	defaultBufferSize = 16 * 1024
)

type Connection struct {
	id            string
	secretKey     []byte
	conn          multiplex.Session
	seq           uint16
	info          *NodeInfo
	dialer        net.Dialer
	goroutinePool *goroutinePool.Pool
	concurrency   int32
	sequence      uint16
	closeFlag     int32
	closeChan     chan struct{}
}

func (conn *Connection) ID() string {
	return conn.id
}

func (conn *Connection) handshake(stream multiplex.Stream) (rwc io.ReadWriteCloser, err error) {
	var (
		frame   *packet.Frame
		timeout time.Duration
	)
	if frame, err = packet.ReadFrame(stream); err != nil {
		return
	}
	if frame.Type != packet.TypeHandshakeRequest {
		return nil, fmt.Errorf("unsupported frame type %v", frame.Type)
	}
	req := &packet.HandshakeRequest{}
	if err = json.Unmarshal(frame.Buf, req); err != nil {
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
		err = packet.WriteFrame(stream, packet.NewFrame(packet.TypeHandshakeResponse, frame.Sequence, res))
	} else {
		res.Reason = err.Error()
		_ = packet.WriteFrame(stream, packet.NewFrame(packet.TypeHandshakeResponse, frame.Sequence, res))
	}
	return
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

func (conn *Connection) process(local multiplex.Stream) {
	var (
		err    error
		remote io.ReadWriteCloser
	)
	atomic.AddInt32(&conn.concurrency, 1)
	defer func() {
		err = local.Close()
		atomic.AddInt32(&conn.concurrency, -1)
	}()
	if remote, err = conn.handshake(local); err != nil {
		return
	}
	defer func() {
		err = remote.Close()
	}()
	errChan := make(chan error, 2)
	conn.goroutinePool.Go(func() {
		conn.pipe(remote, local, errChan)
	})
	conn.goroutinePool.Go(func() {
		conn.pipe(local, remote, errChan)
	})
	select {
	case <-conn.closeChan:
		err = io.ErrClosedPipe
	case err = <-errChan:
	}
	return
}

func (conn *Connection) Ping(ctx context.Context) (err error) {
	var (
		stream multiplex.Stream
		reply  *packet.Frame
	)
	if stream, err = conn.conn.OpenStream(ctx); err != nil {
		return
	}
	defer func() {
		err = stream.Close()
		if r := recover(); r != nil {
			log.Warnf("ping panic: %v: %s", r, string(debug.Stack()))
		}
	}()
	if conn.sequence >= math.MaxUint16 {
		conn.sequence = 0
	}
	conn.sequence++
	if reply, err = packet.SendRecv(ctx, stream, packet.NewFrame(
		packet.TypeHandshakePing,
		conn.sequence,
		packet.PingRequest{
			Timestamp: time.Now().Unix(),
		}),
	); err == nil {
		if reply.Type == packet.TypeHandshakePong {
			return nil
		}
	}
	return
}

func (conn *Connection) Dial(ctx context.Context, proto, addr string) (err error) {
	var (
		buf   []byte
		frame *packet.Frame
	)
	switch proto {
	case "quic":
		conn.conn, err = quic.Dial(ctx, addr)
	case "kcp":
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
	conn.seq++
	req := &packet.RegisterRequest{
		ID:      conn.info.ID,
		OS:      runtime.GOOS,
		Name:    conn.info.Name,
		Country: conn.info.Country,
		IP:      conn.info.IP,
		CPU:     conn.info.CPU,
		Uptime:  conn.info.Uptime,
	}
	if err = conn.conn.WriteMessage(packet.NewFrame(packet.TypeRegisterRequest, conn.seq, req).Bytes()); err != nil {
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
	log.Debugf("connection %s closing", conn.id)
	close(conn.closeChan)
	conn.goroutinePool.Wait()
	err = conn.conn.Close()
	log.Debugf("connection %s closed", conn.id)
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
		id:            xid.New().String(),
		info:          info,
		secretKey:     secretKey,
		goroutinePool: goroutinePool.New().WithMaxGoroutines(2048),
		closeChan:     make(chan struct{}),
	}
}
