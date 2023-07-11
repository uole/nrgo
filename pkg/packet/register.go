package packet

import "time"

const (
	TypeRegisterRequest  = 0x03
	TypeRegisterResponse = 0x04
)

type (
	RegisterRequest struct {
		ID      string    `json:"id"`
		TID     string    `json:"tid"`
		Name    string    `json:"name"`
		OS      string    `json:"os"`
		Country string    `json:"country"`
		IP      string    `json:"ip"`
		CPU     int       `json:"cpu"`
		Secret  string    `json:"secret"`
		Uptime  time.Time `json:"uptime"`
	}

	RegisterResponse struct {
		ID      string `json:"id"`
		Success bool   `json:"success"`
		Reason  string `json:"reason"`
	}
)
