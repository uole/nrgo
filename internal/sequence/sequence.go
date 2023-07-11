package sequence

import (
	"math"
	"sync"
)

var (
	sequence uint16
	mutex    sync.Mutex
)

func Next() uint16 {
	mutex.Lock()
	defer mutex.Unlock()
	if sequence >= math.MaxUint16 {
		sequence = 0
	}
	sequence++
	return sequence
}
