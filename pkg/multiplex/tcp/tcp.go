package tcp

import (
	"context"
	"github.com/uole/nrgo/pkg/multiplex"
	"github.com/uole/nrgo/pkg/stream"
	"net"
)

func Listen(addr string, cbs ...Option) (multiplex.Listener, error) {
	var (
		err    error
		listen net.Listener
	)
	opts := &Options{}
	for _, cb := range cbs {
		cb(opts)
	}
	if listen, err = net.Listen("tcp", addr); err != nil {
		return nil, err
	} else {
		return &Listener{l: listen, opts: opts}, nil
	}
}

func Dial(ctx context.Context, addr string, cbs ...Option) (multiplex.Session, error) {
	var (
		err    error
		conn   net.Conn
		dialer net.Dialer
	)
	opts := &Options{}
	for _, cb := range cbs {
		cb(opts)
	}
	if conn, err = dialer.DialContext(ctx, "tcp", addr); err != nil {
		return nil, err
	} else {
		if opts.Key != nil {
			return newSession(stream.New(conn, stream.WithEncrypt(opts.Key))), nil
		} else {
			return newSession(conn), nil
		}
	}
}
