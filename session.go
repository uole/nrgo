package nrgo

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"git.nspix.com/golang/kos/pkg/log"
	"sync"
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
	ID                 string
	State              int32
	Connecting         int32
	info               *NodeInfo
	handshake          Handshake
	connections        *sync.Map
	connectionInfo     *ConnectionInfo
	connectionInfoHash string
	updateMutex        sync.Mutex
}

func (sess *Session) updateConnectionInfo(ctx context.Context, ci *ConnectionInfo) (err error) {
	sess.updateMutex.Lock()
	defer sess.updateMutex.Unlock()
	hash := md5.New()
	if err = json.NewEncoder(hash).Encode(ci); err != nil {
		return
	}
	str := hex.EncodeToString(hash.Sum(nil))
	if str != sess.connectionInfoHash {
		sess.connectionInfoHash = str
		sess.connectionInfo = ci
	}
	sess.connections.Range(func(key, value any) bool {
		conn := value.(*Connection)
		err = conn.Close()
		return true
	})
	err = sess.Serve(ctx)
	return
}

func (sess *Session) IsEqual(state int32) bool {
	return atomic.LoadInt32(&sess.State) == state
}

func (sess *Session) startConnection(ctx context.Context, index int) {
	var (
		err   error
		tires int32
		conn  *Connection
	)
	tires = 1
__connection:
	conn = newConnection(fmt.Sprintf("%s%02d", sess.ID, index), sess.connectionInfo.SecretKey, sess.info, sess.handshake)
	log.Debugf("session %s connection %d@%s connecting %s://%s", sess.ID, index, conn.id, sess.connectionInfo.Proto, sess.connectionInfo.Address.Tunnel)
	if err = conn.Dial(ctx, sess.connectionInfo.Proto, sess.connectionInfo.Address.Tunnel); err != nil {
		log.Warnf("session %s connection %d@%s dial %s error: %s", sess.ID, index, conn.id, sess.connectionInfo.Address.Tunnel, err.Error())
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Second * time.Duration(tires*5)):
			tires++
			log.Debugf("try reconnect session %s connection %d %s", sess.ID, index, sess.connectionInfo.Address.Tunnel)
		}
		goto __connection
	} else {
		tires = 1
	}
	sess.connections.Store(index, conn)
	log.Infof("session %s connection %s@%d connected %s", sess.ID, conn.id, index, sess.connectionInfo.Address.Tunnel)
	if err = conn.Serve(ctx); err != nil {
		log.Warnf("session %s connection %s@%d serve error: %s", sess.ID, conn.id, index, err.Error())
	}
	if !conn.Closed() {
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Second * time.Duration(tires*5)):
			err = conn.Close()
			tires++
			log.Debugf("session %s connection %d try reconnect %s", sess.ID, index, sess.connectionInfo.Address.Tunnel)
		}
		goto __connection
	}
	log.Infof("session %s connection %d is closed", sess.ID, index)
	return
}

func (sess *Session) Serve(ctx context.Context) (err error) {
	if !atomic.CompareAndSwapInt32(&sess.Connecting, 0, 1) {
		return ErrConnecting
	}
	defer func() {
		atomic.StoreInt32(&sess.Connecting, 0)
	}()
	for i := 0; i < sess.connectionInfo.ConnSize; i++ {
		go sess.startConnection(ctx, i+1)
	}
	return
}

func (sess *Session) Close() (err error) {
	sess.connections.Range(func(key, value any) bool {
		conn := value.(*Connection)
		if err = conn.Close(); err != nil {
			log.Warnf("session %s close connection %v@%s error: %s", sess.ID, key, conn.id)
		}
		return true
	})
	return
}

func newSession(id string, ci *ConnectionInfo, info *NodeInfo, handshake Handshake) (sess *Session) {
	sess = &Session{
		ID:             id,
		info:           info,
		connectionInfo: ci,
		connections:    new(sync.Map),
		handshake:      handshake,
		State:          StateUnavailable,
	}
	return sess
}
