package packet

import "time"

const (
	TypeHandshakeRequest  = 0x0A
	TypeHandshakeResponse = 0x0B
)

type (
	HandshakeRequest struct {
		Host    string        `json:"host"`
		Timeout time.Duration `json:"timeout"`
	}

	HandshakeResponse struct {
		Host    string `json:"host"`
		Success bool   `json:"success"`
		Reason  string `json:"reason,omitempty"`
	}
)
