package utils

import (
	"crypto/sha1"
	"encoding/hex"
	"github.com/templexxx/xorsimd"
	"golang.org/x/crypto/pbkdf2"
)

var (
	Salt = []byte("vrgo")
)

func cryptKey() []byte {
	return pbkdf2.Key(Salt, Salt, 2, 256, sha1.New)
}

func EncryptSecret(s string) string {
	buf := []byte(s)
	xorsimd.Bytes(buf, buf, cryptKey())
	return hex.EncodeToString(buf)
}

func DecryptSecret(s string) (string, error) {
	var (
		err error
		buf []byte
	)
	if buf, err = hex.DecodeString(s); err != nil {
		return "", err
	}
	xorsimd.Bytes(buf, buf, cryptKey())
	return string(buf), nil
}
