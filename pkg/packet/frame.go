package packet

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"git.nspix.com/golang/kos/util/pool"
	"io"
	"math"
)

const (
	Var = 0xBB

	frameHeadLength = 6
)

type Frame struct {
	Ver      uint8
	Type     uint8
	Sequence uint16
	Length   uint16
	Buf      []byte
}

func (f *Frame) Bytes() []byte {
	nl := len(f.Buf)
	buf := make([]byte, frameHeadLength+nl)
	buf[0] = Var
	buf[1] = f.Type
	f.Length = uint16(nl)
	binary.BigEndian.PutUint16(buf[2:], f.Sequence)
	binary.BigEndian.PutUint16(buf[4:], f.Length)
	copy(buf[6:], f.Buf[:])
	return buf
}

func ReadFrame(r io.Reader) (f *Frame, err error) {
	var (
		n int
	)
	head := pool.GetBytes(frameHeadLength)
	defer func() {
		pool.PutBytes(head)
	}()
	if n, err = io.ReadFull(r, head); err != nil {
		return
	}
	f = &Frame{
		Ver:  head[0],
		Type: head[1],
	}
	if f.Ver != Var {
		return nil, fmt.Errorf("invalid frame ver %0x", f.Ver)
	}
	f.Sequence = binary.BigEndian.Uint16(head[2:])
	f.Length = binary.BigEndian.Uint16(head[4:])
	if f.Length > 0 {
		f.Buf = make([]byte, f.Length)
	}
	if n, err = io.ReadFull(r, f.Buf); err == nil {
		if n < int(f.Length) {
			err = io.ErrShortBuffer
		}
	}
	return
}

func WriteFrame(w io.Writer, f *Frame) (err error) {
	var (
		n  int
		nw int64
	)
	n = len(f.Buf)
	if n > math.MaxUint16 {
		return io.ErrNoProgress
	}
	f.Length = uint16(n)
	buf := pool.GetBuffer()
	defer func() {
		defer pool.PutBuffer(buf)
	}()
	buf.WriteByte(Var)
	buf.WriteByte(f.Type)
	if err = binary.Write(buf, binary.BigEndian, f.Sequence); err != nil {
		return
	}
	if err = binary.Write(buf, binary.BigEndian, f.Length); err != nil {
		return
	}
	if f.Buf != nil {
		buf.Write(f.Buf)
	}
	if nw, err = buf.WriteTo(w); err == nil {
		if nw < int64(frameHeadLength+f.Length) {
			err = io.ErrShortWrite
		}
	}
	return
}

func SendRecv(rw io.ReadWriter, f *Frame) (res *Frame, err error) {
	if err = WriteFrame(rw, f); err != nil {
		return
	}
	if res, err = ReadFrame(rw); err != nil {
		return
	}
	if res.Sequence != f.Sequence {
		err = fmt.Errorf("recv frame sequence not equal %v", f.Sequence)
	}
	return
}

func NewFrame(action uint8, seq uint16, v any) *Frame {
	var (
		buf []byte
	)
	if v != nil {
		buf, _ = json.Marshal(v)
	}
	return &Frame{
		Ver:      Var,
		Type:     action,
		Sequence: seq,
		Buf:      buf,
	}
}
