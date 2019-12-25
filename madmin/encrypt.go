package madmin

import (
	"bytes"
	"crypto/rand"
	"io"
	"io/ioutil"

	"github.com/minio/sio"
	"golang.org/x/crypto/argon2"
)

// EncryptData - encrypts server config data.
func EncryptData(password string, data []byte) ([]byte, error) {
	salt := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, err
	}

	// derive an encryption key from the master key and the nonce
	var key [32]byte
	copy(key[:], argon2.IDKey([]byte(password), salt, 1, 64*1024, 4, 32))

	encrypted, err := sio.EncryptReader(bytes.NewReader(data), sio.Config{
		Key: key[:]},
	)
	if err != nil {
		return nil, err
	}
	edata, err := ioutil.ReadAll(encrypted)
	return append(salt, edata...), err
}

// DecryptData - decrypts server config data.
func DecryptData(password string, data io.Reader) ([]byte, error) {
	salt := make([]byte, 32)
	if _, err := io.ReadFull(data, salt); err != nil {
		return nil, err
	}
	// derive an encryption key from the master key and the nonce
	var key [32]byte
	copy(key[:], argon2.IDKey([]byte(password), salt, 1, 64*1024, 4, 32))

	decrypted, err := sio.DecryptReader(data, sio.Config{
		Key: key[:]},
	)
	if err != nil {
		return nil, err
	}
	return ioutil.ReadAll(decrypted)
}
