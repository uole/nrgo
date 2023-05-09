package quic

import "github.com/quic-go/quic-go"

type Option func(cfg *quic.Config)
