package config

import "github.com/uole/nrgo"

type Config struct {
	Direct    bool                 `yaml:"direct" yaml:"direct"`
	Wireguard nrgo.WireguardConfig `json:"warp" yaml:"warp"`
}

func New() *Config {
	return &Config{Direct: true}
}
