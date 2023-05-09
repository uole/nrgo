package kcp

import (
	"context"
	"github.com/uole/nrgo/pkg/multiplex"
	kcp "github.com/xtaci/kcp-go"
	"net"
)

func Listen(addr string, cbs ...Option) (multiplex.Listener, error) {
	var (
		err    error
		listen *kcp.Listener
		block  kcp.BlockCrypt
	)
	opts := &Options{}
	for _, cb := range cbs {
		cb(opts)
	}
	if block, err = kcp.NewSimpleXORBlockCrypt(opts.Key); err != nil {
		return nil, err
	}
	if listen, err = kcp.ListenWithOptions(addr, block, 10, 3); err != nil {
		return nil, err
	} else {
		return &Listener{l: listen}, nil
	}
}

func Dial(ctx context.Context, addr string, cbs ...Option) (multiplex.Session, error) {
	var (
		err   error
		conn  net.Conn
		block kcp.BlockCrypt
	)
	opts := &Options{}
	for _, cb := range cbs {
		cb(opts)
	}
	if block, err = kcp.NewSimpleXORBlockCrypt(opts.Key); err != nil {
		return nil, err
	}
	if conn, err = kcp.DialWithOptions(addr, block, 10, 3); err != nil {
		return nil, err
	} else {
		return newSession(conn), nil
	}
}
