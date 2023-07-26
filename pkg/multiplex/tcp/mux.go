package tcp

import (
	"context"
	"git.nspix.com/golang/kos/pkg/log"
	"git.nspix.com/golang/yamux"
	"github.com/uole/nrgo/internal/crypto"
	"github.com/uole/nrgo/pkg/multiplex"
	"io"
	"net"
	"sync"
	"sync/atomic"
)

type (
	Listener struct {
		l    net.Listener
		opts *Options
	}

	Session struct {
		conn     net.Conn
		sess     *yamux.Session
		once     sync.Once
		key      []byte
		takeover int32
	}
)

func (sess *Session) initSession() {
	var (
		err error
	)
	cfg := yamux.DefaultConfig()
	cfg.MaxStreamWindowSize = 512 * 1024
	cfg.Logger = log.GetLogger()
	cfg.LogOutput = nil
	if sess.key != nil {
		cfg.Crypto = crypto.NewXorEncrypt(sess.key)
	}
	if sess.sess, err = yamux.Server(sess.conn, cfg); err != nil {
		panic("create mux error:" + err.Error())
	}
	atomic.StoreInt32(&sess.takeover, 1)
}

func (sess *Session) OpenStream(ctx context.Context) (multiplex.Stream, error) {
	sess.once.Do(sess.initSession)
	return sess.sess.OpenStream()
}

func (sess *Session) AcceptStream(ctx context.Context) (multiplex.Stream, error) {
	sess.once.Do(sess.initSession)
	return sess.sess.AcceptStreamWithContext(ctx)
}

func (sess *Session) ReadMessage() ([]byte, error) {
	var (
		n   int
		err error
	)
	if atomic.LoadInt32(&sess.takeover) == 1 {
		return nil, io.ErrNoProgress
	}
	buf := make([]byte, 1024*4)
	if n, err = sess.conn.Read(buf); err == nil {
		return buf[:n], nil
	} else {
		return nil, err
	}
}

func (sess *Session) WriteMessage(bytes []byte) error {
	if atomic.LoadInt32(&sess.takeover) == 1 {
		return io.ErrNoProgress
	}
	if _, err := sess.conn.Write(bytes); err != nil {
		return err
	} else {
		return nil
	}
}

func (sess *Session) Addr() net.Addr {
	return sess.conn.RemoteAddr()
}

func (sess *Session) Close() error {
	if sess.sess != nil {
		return sess.sess.Close()
	} else {
		return sess.conn.Close()
	}
}

func newSession(conn net.Conn, key []byte) *Session {
	sess := &Session{conn: conn, key: key}
	return sess
}

func (l *Listener) Accept(ctx context.Context) (multiplex.Session, error) {
	if conn, err := l.l.Accept(); err != nil {
		return nil, err
	} else {
		if l.opts.Key == nil {
			return newSession(conn, nil), nil
		} else {
			return newSession(conn, l.opts.Key), nil
		}
	}
}

func (l *Listener) Close() (err error) {
	return l.l.Close()
}
