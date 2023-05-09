package tcp

type Options struct {
	Key []byte
}

type Option func(opts *Options)
