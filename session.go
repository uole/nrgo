package nrgo

import (
	"context"
	"errors"
	"fmt"
	"git.nspix.com/golang/kos/pkg/log"
	"github.com/quic-go/quic-go"
	"github.com/uole/nrgo/pkg/packet"
	"math"
	"sync/atomic"
	"time"
)

const (
	StatePadding     = 0x01
	StateReady       = 0x02
	StateUnavailable = 0x03
)

var (
	ErrConnecting = errors.New("already connecting")
)

type Session struct {
	ID              string
	Address         string
	State           int32
	Connecting      int32
	HeartbeatTime   time.Time
	Tires           int32
	LastAttemptTime time.Time
	info            *NodeInfo
	conn            *Connection
	sequence        uint16
	restartFlag     int32
}

func (sess *Session) Ping(ctx context.Context) {
	var (
		err    error
		stream quic.Stream
		reply  *packet.Frame
	)
	if atomic.LoadInt32(&sess.State) != StateReady {
		return
	}
	if stream, err = sess.conn.conn.OpenStreamSync(ctx); err != nil {
		return
	}
	defer func() {
		err = stream.Close()
	}()
	if sess.sequence >= math.MaxUint16 {
		sess.sequence = 0
	}
	sess.sequence++
	if reply, err = packet.SendRecv(stream, packet.NewFrame(packet.TypeHandshakePing, sess.sequence, packet.PingRequest{Timestamp: time.Now().Unix()})); err == nil {
		if reply.Type == packet.TypeHandshakePong {
			sess.HeartbeatTime = time.Now()
		}
	}
}

func (sess *Session) updateAddress(address string) {
	//对信息进行重置
	if sess.Address != address {
		sess.Address = address
		atomic.StoreInt32(&sess.Tires, 0)
		_ = sess.Close() //reconnect
	}
}

func (sess *Session) IsEqual(state int32) bool {
	return atomic.LoadInt32(&sess.State) == state
}

func (sess *Session) Connect(ctx context.Context) (err error) {
	if !atomic.CompareAndSwapInt32(&sess.Connecting, 0, 1) {
		return ErrConnecting
	}
	defer atomic.StoreInt32(&sess.Connecting, 0)
	duration := time.Now().Sub(sess.LastAttemptTime)
	if duration.Minutes() < math.Pow(float64(sess.Tires), 2) {
		return fmt.Errorf("%s are left until the next connection", duration)
	}
	atomic.AddInt32(&sess.Tires, 1)
	sess.State = StatePadding
	sess.conn = NewConnection(sess.info)
	if err = sess.conn.Dial(ctx, sess.Address); err == nil {
		sess.State = StateReady
		atomic.StoreInt32(&sess.Tires, 0)
	} else {
		sess.LastAttemptTime = time.Now()
	}
	return
}

func (sess *Session) Receive(ctx context.Context) {
	sess.conn.IoLoop(ctx)
	atomic.StoreInt32(&sess.State, StateUnavailable)
	log.Infof("session %s closed", sess.ID)
}

func (sess *Session) Close() (err error) {
	atomic.StoreInt32(&sess.State, StateUnavailable)
	return sess.conn.Close()
}

func newSession(id string, addr string, info *NodeInfo) (sess *Session) {
	sess = &Session{
		ID:            id,
		Address:       addr,
		info:          info,
		HeartbeatTime: time.Now(),
	}
	return sess
}
