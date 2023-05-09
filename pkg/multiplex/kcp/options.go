package kcp

type Options struct {
	Key []byte
}

type Option func(opts *Options)
