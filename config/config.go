package config

type (
	Interface struct {
		PrivateKey string `json:"private_key" yaml:"privateKey"`
		Address    string `json:"address" yaml:"address"`
		DNS        string `json:"dns" yaml:"dns"`
		MTU        uint   `json:"mtu" yaml:"mtu"`
	}

	Peer struct {
		PublicKey  string `json:"public_key" yaml:"publicKey"`
		AllowedIPs string `json:"allowed_ips" yaml:"allowedIPs"`
		Endpoint   string `json:"endpoint" yaml:"endpoint"`
	}

	Wireguard struct {
		Interface Interface `json:"interface" yaml:"interface"`
		Peer      Peer      `json:"peer" yaml:"peer"`
	}
)

type Config struct {
	Direct    bool      `yaml:"direct" yaml:"direct"`
	Wireguard Wireguard `json:"warp" yaml:"warp"`
}

func New() *Config {
	return &Config{Direct: true}
}
