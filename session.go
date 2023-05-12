package nrgo

import (
	"bytes"
	"context"
	"errors"
	"git.nspix.com/golang/kos/pkg/log"
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
	Tires           int32
	LastAttemptTime time.Time
	secretKey       []byte
	info            *NodeInfo
	conn            *Connection
	sequence        uint16
	restartFlag     int32
}

func (sess *Session) updateSecretKey(key []byte) {
	if bytes.Compare(key, sess.secretKey) != 0 {
		sess.secretKey = make([]byte, len(key))
		copy(sess.secretKey[:], key[:])
		atomic.StoreInt32(&sess.Tires, 0)
		_ = sess.Close()
		log.Debugf("session %s secret key updated", sess.ID)
	}
}

func (sess *Session) updateAddress(proto string, address string) {
	if sess.Proto != proto || sess.Address != address {
		sess.Proto = proto
		sess.Address = address
		atomic.StoreInt32(&sess.Tires, 0)
		_ = sess.Close()
		log.Debugf("session %s address updated", sess.ID)
	}
}

func (sess *Session) IsEqual(state int32) bool {
	return atomic.LoadInt32(&sess.State) == state
}

func (sess *Session) Connect(ctx context.Context) (err error) {
	var (
		conn *Connection
	)
	if !atomic.CompareAndSwapInt32(&sess.Connecting, 0, 1) {
		return ErrConnecting
	}
	defer func() {
		atomic.StoreInt32(&sess.Connecting, 0)
	}()
	atomic.AddInt32(&sess.Tires, 1)
	sess.State = StatePadding
	conn = NewConnection(sess.secretKey, sess.info)
	if err = conn.Dial(ctx, sess.Proto, sess.Address); err != nil {
		sess.State = StateUnavailable
		sess.LastAttemptTime = time.Now()
		log.Debugf("session %s connect with %s error: %s", sess.ID, sess.Proto, err.Error())
		return
	}
	atomic.StoreInt32(&sess.Tires, 0)
	if sess.conn != nil {
		oldConn := sess.conn
		if oldConn.ID() != conn.ID() {
			log.Debugf("session %s replace connection %s -> %s", sess.ID, oldConn.ID(), conn.ID())
		} else {
			log.Debugf("session %s attach connection %s", sess.ID, conn.ID())
		}
		if err = oldConn.Close(); err != nil {
			log.Warnf("session %s closed old connection %s error: %s", sess.ID, oldConn.ID(), err.Error())
		}
	}
	sess.conn = conn
	sess.State = StateReady
	log.Debugf("session %s connected with %s protocol", sess.ID, sess.Proto)
	go sess.Receive(ctx, conn)
	return
}

func (sess *Session) Receive(ctx context.Context, conn *Connection) {
	if err := conn.IoLoop(ctx); err != nil {
		log.Warnf("session %s io error: %s", sess.ID, err.Error())
		err = sess.Close()
	}
}

func (sess *Session) Close() (err error) {
	if atomic.CompareAndSwapInt32(&sess.State, StateReady, StateUnavailable) {
		if err = sess.conn.Close(); err != nil {
			log.Warnf("session %s connection %s close error: %s", sess.ID, err.Error())
		}
		log.Debugf("session %s closed", sess.ID)
	}
	return
}

func newSession(id string, proto, addr string, secretKey []byte, info *NodeInfo) (sess *Session) {
	sess = &Session{
		ID:        id,
		Proto:     proto,
		Address:   addr,
		info:      info,
		State:     StateUnavailable,
		secretKey: secretKey,
	}
	return sess
}
