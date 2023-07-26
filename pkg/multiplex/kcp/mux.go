package kcp

import (
	"context"
	"github.com/uole/nrgo/pkg/multiplex"
	"github.com/uole/smux"
	kcp "github.com/xtaci/kcp-go"
	"io"
	"net"
	"sync"
	"sync/atomic"
)

type (
	Listener struct {
		l *kcp.Listener
	}
	Session struct {
		conn     net.Conn
		sess     *smux.Session
		once     sync.Once
		takeover int32
	}
)

func (sess *Session) initSession() {
	cfg := smux.DefaultConfig()
	sess.sess, _ = smux.Server(sess.conn, cfg)
	atomic.StoreInt32(&sess.takeover, 1)
}

func (sess *Session) OpenStream(ctx context.Context) (multiplex.Stream, error) {
	sess.once.Do(sess.initSession)
	return sess.sess.OpenStream(ctx)
}

func (sess *Session) AcceptStream(ctx context.Context) (multiplex.Stream, error) {
	sess.once.Do(sess.initSession)
	return sess.sess.AcceptStream(ctx)
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

func newSession(conn net.Conn) *Session {
	sess := &Session{conn: conn}
	return sess
}

func (l *Listener) Accept(ctx context.Context) (multiplex.Session, error) {
	if conn, err := l.l.Accept(); err != nil {
		return nil, err
	} else {
		return newSession(conn), nil
	}
}

func (l *Listener) Close() (err error) {
	return l.l.Close()
}
