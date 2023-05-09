package nrgo

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"git.nspix.com/golang/kos/pkg/log"
	"io"
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
	Proto           string
	HeartbeatTime   time.Time
	Tires           int32
	LastAttemptTime time.Time
	secretKey       []byte
	info            *NodeInfo
	conn            *Connection
	sequence        uint16
	restartFlag     int32
}

func (sess *Session) Ping(ctx context.Context) {
	var (
		err error
	)
	if atomic.LoadInt32(&sess.State) != StateReady {
		return
	}
	if err = sess.conn.Ping(ctx); err == nil {
		sess.HeartbeatTime = time.Now()
	} else {
		log.Debugf("session %s handshake ping message error: %s", sess.ID, err.Error())
		if errors.Is(err, io.ErrClosedPipe) {
			err = sess.Close()
		}
	}
}

func (sess *Session) updateSecretKey(key []byte) {
	if bytes.Compare(key, sess.secretKey) != 0 {
		sess.secretKey = make([]byte, len(key))
		copy(sess.secretKey[:], key[:])
		atomic.StoreInt32(&sess.Tires, 0)
		_ = sess.Close()
	}
}

func (sess *Session) updateAddress(proto string, address string) {
	if sess.Proto != proto || sess.Address != address {
		sess.Proto = proto
		sess.Address = address
		atomic.StoreInt32(&sess.Tires, 0)
		_ = sess.Close()
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
	sess.conn = NewConnection(sess.secretKey, sess.info)
	if err = sess.conn.Dial(ctx, sess.Proto, sess.Address); err == nil {
		sess.State = StateReady
		atomic.StoreInt32(&sess.Tires, 0)
		log.Debugf("dial %s with %s successful", sess.Proto, sess.Address)
	} else {
		sess.LastAttemptTime = time.Now()
		log.Debugf("dial %s with %s error: %s", sess.Proto, sess.Address, err.Error())
	}
	return
}

func (sess *Session) Receive(ctx context.Context) {
	sess.conn.IoLoop(ctx)
	atomic.StoreInt32(&sess.State, StateUnavailable)
	log.Infof("session %s closed", sess.ID)
}

func (sess *Session) Close() (err error) {
	if atomic.CompareAndSwapInt32(&sess.State, StateReady, StateUnavailable) {
		err = sess.conn.Close()
	}
	return
}

func newSession(id string, proto, addr string, secretKey []byte, info *NodeInfo) (sess *Session) {
	sess = &Session{
		ID:            id,
		Proto:         proto,
		Address:       addr,
		info:          info,
		secretKey:     secretKey,
		HeartbeatTime: time.Now(),
	}
	return sess
}
