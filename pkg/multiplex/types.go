package multiplex

import (
	"context"
	"io"
	"net"
)

type (
	Listener interface {
		Accept(ctx context.Context) (Session, error)
		Close() (err error)
	}

	Session interface {
		Addr() net.Addr
		OpenStream(ctx context.Context) (Stream, error)
		AcceptStream(ctx context.Context) (Stream, error)
		ReadMessage() ([]byte, error)
		WriteMessage([]byte) error
		Close() error
	}

	Stream interface {
		io.ReadWriteCloser
	}
)
