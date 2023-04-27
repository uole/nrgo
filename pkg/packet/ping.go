package packet

const (
	TypeHandshakePing = 0x05
	TypeHandshakePong = 0x06
)

type (
	PingRequest struct {
		Timestamp int64
	}

	PongResponse struct {
		Timestamp int64
	}
)
