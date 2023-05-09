package quic

import (
	"context"
	"github.com/quic-go/quic-go"
	"github.com/uole/nrgo/pkg/multiplex"
	"io"
	"net"
)

type (
	Listener struct {
		l quic.Listener
	}

	Session struct {
		conn quic.Connection
	}
)

func (sess *Session) ReadMessage() ([]byte, error) {
	return sess.conn.ReceiveMessage()
}

func (sess *Session) WriteMessage(bytes []byte) error {
	return sess.conn.SendMessage(bytes)
}

func (sess *Session) Addr() net.Addr {
	return sess.conn.RemoteAddr()
}

func (sess *Session) OpenStream(ctx context.Context) (multiplex.Stream, error) {
	return sess.conn.OpenStreamSync(ctx)
}

func (sess *Session) AcceptStream(ctx context.Context) (multiplex.Stream, error) {
	return sess.conn.AcceptStream(ctx)
}

func (sess *Session) Close() error {
	return sess.conn.CloseWithError(1000, io.ErrClosedPipe.Error())
}

func (mux Listener) Accept(ctx context.Context) (multiplex.Session, error) {
	if conn, err := mux.l.Accept(ctx); err == nil {
		return &Session{conn: conn}, nil
	} else {
		return nil, err
	}
}

func (mux *Listener) Close() error {
	return mux.l.Close()
}
