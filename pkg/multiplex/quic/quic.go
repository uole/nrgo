package quic

import (
	"context"
	"crypto/tls"
	"github.com/quic-go/quic-go"
	"github.com/uole/nrgo/pkg/multiplex"
	"os"
	"time"
)

func init() {
	os.Setenv("QUIC_GO_DISABLE_RECEIVE_BUFFER_WARNING", "true")
	os.Setenv("QUIC_GO_LOG_LEVEL", "error")
}

func Dial(ctx context.Context, addr string, cbs ...Option) (multiplex.Session, error) {
	var (
		err  error
		conn quic.Connection
	)
	cfg := &quic.Config{
		MaxIdleTimeout:        time.Second * 80,
		EnableDatagrams:       true,
		MaxIncomingStreams:    1024,
		MaxIncomingUniStreams: 1024,
	}
	for _, cb := range cbs {
		cb(cfg)
	}
	if conn, err = quic.DialAddrContext(ctx, addr, &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{"DESeOlBg1dK"},
	}, cfg); err != nil {
		return nil, err
	} else {
		return &Session{conn: conn}, nil
	}
}
