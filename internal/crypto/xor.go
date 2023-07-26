package crypto

import (
	"crypto/sha1"
	"github.com/templexxx/xorsimd"
	"golang.org/x/crypto/pbkdf2"
)

var (
	xorKeySalt = []byte{0xFB, 0xFA, 0xFF}
)

const (
	xorBlockSize = 1024
)

type XOR struct {
	Key []byte
}

func (crypto *XOR) round(buf []byte) int {
	var (
		offset int
		limit  int
		result int
	)
	if len(crypto.Key) < xorBlockSize {
		return 0
	}
	chunkSize := (len(buf) + xorBlockSize) / xorBlockSize
	for i := 0; i < chunkSize; i++ {
		offset = i * xorBlockSize
		limit = offset + xorBlockSize
		if i == chunkSize-1 {
			result += xorsimd.Bytes(buf[offset:], buf[offset:], crypto.Key)
		} else {
			result += xorsimd.Bytes(buf[offset:], buf[offset:limit], crypto.Key)
		}
	}
	return result
}

func (crypto *XOR) Encrypt(src []byte) (dst []byte, err error) {
	crypto.round(src)
	return src, nil
}

func (crypto *XOR) Decrypt(src []byte) (dst []byte, err error) {
	crypto.round(src)
	return src, nil
}

func NewXorEncrypt(key []byte) *XOR {
	return &XOR{
		Key: pbkdf2.Key(key, xorKeySalt, 4, xorBlockSize, sha1.New),
	}
}
