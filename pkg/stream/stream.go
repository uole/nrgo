package stream

import (
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"errors"
	"fmt"
	"git.nspix.com/golang/kos/util/pool"
	"github.com/golang/snappy"
	"github.com/templexxx/xorsimd"
	"golang.org/x/crypto/pbkdf2"
	"io"
	"math"
	"math/rand"
	"net"
	"sync/atomic"
	"time"
)

var (
	saltxor = []byte{0x01, 0x25, 0x68, 0x79}
)

const (
	typeEncryption = 0x0040
	typeCompress   = 0x0080

	BlockSize = 1024

	Ver               = 0xFB
	minCompressLength = 512
)

type (
	Conn struct {
		opts      *Options
		rw        io.ReadWriter
		buf       *bytes.Buffer
		closeFlag int32
	}

	Option func(o *Options)

	Options struct {
		Compress   bool
		Encrypt    bool
		EncryptKey []byte
	}
)

func crypto(buf []byte, key []byte) {
	var (
		limit  int
		offset int
	)
	l := len(buf)
	if l < BlockSize {
		xorsimd.Bytes(buf, buf, key)
		return
	}
	chunkSize := int(math.Ceil(float64(l) / float64(BlockSize)))
	for i := 0; i < chunkSize; i++ {
		offset = i * BlockSize
		limit = offset + BlockSize
		if i == chunkSize-1 {
			xorsimd.Bytes(buf[offset:], buf[offset:], key)
		} else {
			xorsimd.Bytes(buf[offset:limit], buf[offset:limit], key)
		}
	}
}

func (conn *Conn) tryRead() (err error) {
	var (
		n    int
		flag uint8
		head []byte
		src  []byte
		dst  []byte
		p    []byte
	)
	head = pool.GetBytes(6)
	defer pool.PutBytes(head)
	if n, err = io.ReadFull(conn.rw, head); err != nil {
		return
	}
	if head[0] != Ver {
		err = fmt.Errorf("invalid stream protocol version 0x%02X", head[0])
		return
	}
	flag = head[1]
	src = pool.GetBytes(int(binary.BigEndian.Uint32(head[2:])))
	defer pool.PutBytes(src)
	if n, err = io.ReadFull(conn.rw, src); err != nil {
		return
	}
	//encrypted
	if ((flag >> 6) & 1) == 1 {
		crypto(src, conn.opts.EncryptKey)
	}
	//compressed
	if ((flag >> 7) & 1) == 1 {
		if n, err = snappy.DecodedLen(src); err != nil {
			return
		}
		dst = pool.GetBytes(n)
		defer pool.PutBytes(dst)
		p, err = snappy.Decode(dst, src)
	} else {
		p = src
	}
	if err == nil {
		conn.buf.Write(p)
	}
	return
}

func (conn *Conn) Read(b []byte) (n int, err error) {
	if conn.buf.Len() == 0 {
		if err = conn.tryRead(); err != nil {
			return
		}
	}
	if n, err = conn.buf.Read(b); err != nil {
		if errors.Is(err, io.EOF) {
			err = nil
		}
	}
	return
}

func (conn *Conn) Write(b []byte) (n int, err error) {
	var (
		flag uint8
		p    []byte
		nw   int64
		ntw  int
	)
	length := len(b)
	if length <= 0 {
		return
	}
	w := pool.GetBuffer()
	defer pool.PutBuffer(w)
	if err = w.WriteByte(Ver); err != nil {
		return
	}
	if conn.opts.Compress && length > minCompressLength {
		flag |= typeCompress
		buf := pool.GetBytes(snappy.MaxEncodedLen(length))
		defer pool.PutBytes(buf)
		p = snappy.Encode(buf, b)
	} else {
		p = b
	}
	if conn.opts.Encrypt {
		flag |= typeEncryption
		crypto(p, conn.opts.EncryptKey)
	}
	//grant random number
	flag |= uint8(rand.Int31n(63))
	if err = w.WriteByte(flag); err != nil {
		return
	}
	if err = binary.Write(w, binary.BigEndian, uint32(len(p))); err != nil {
		return
	}
	if ntw, err = w.Write(p); err != nil {
		return
	}
	if nw, err = w.WriteTo(conn.rw); err == nil {
		if nw != int64(ntw)+6 {
			err = io.ErrShortWrite
		}
		n = length
	}
	return
}

func (conn *Conn) Close() (err error) {
	if !atomic.CompareAndSwapInt32(&conn.closeFlag, 0, 1) {
		return
	}
	if c, ok := conn.rw.(io.Closer); ok {
		err = c.Close()
	}
	if conn.buf != nil {
		pool.PutBuffer(conn.buf)
	}
	return
}

func (conn *Conn) LocalAddr() net.Addr {
	if c, ok := conn.rw.(net.Conn); ok {
		return c.LocalAddr()
	}
	return nil
}

func (conn *Conn) RemoteAddr() net.Addr {
	if c, ok := conn.rw.(net.Conn); ok {
		return c.RemoteAddr()
	}
	return nil
}

func (conn *Conn) SetDeadline(t time.Time) error {
	if c, ok := conn.rw.(net.Conn); ok {
		return c.SetDeadline(t)
	}
	return nil
}

func (conn *Conn) SetReadDeadline(t time.Time) error {
	if c, ok := conn.rw.(net.Conn); ok {
		return c.SetReadDeadline(t)
	}
	return nil
}

func (conn *Conn) SetWriteDeadline(t time.Time) error {
	if c, ok := conn.rw.(net.Conn); ok {
		return c.SetWriteDeadline(t)
	}
	return nil
}

func WithCompress() Option {
	return func(o *Options) {
		o.Compress = true
	}
}

func WithEncrypt(key []byte) Option {
	return func(o *Options) {
		if len(key) > 0 {
			o.Encrypt = true
			if len(key) == BlockSize {
				o.EncryptKey = key
			} else {
				o.EncryptKey = pbkdf2.Key(key, saltxor, 32, BlockSize, sha1.New)
			}
		} else {
			o.Encrypt = false
		}
	}
}

func New(rw io.ReadWriter, cbs ...Option) *Conn {
	opts := &Options{}
	for _, cb := range cbs {
		cb(opts)
	}
	conn := &Conn{
		rw:   rw,
		opts: opts,
		buf:  pool.GetBuffer(),
	}
	return conn
}
