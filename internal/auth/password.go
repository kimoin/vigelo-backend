package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	argonTime    = 2
	argonMemory  = 64 * 1024
	argonThreads = 4
	argonKeyLen  = 32
	argonSaltLen = 16
)

var ErrInvalidPassword = errors.New("invalid password hash")

func HashPassword(password string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	hash := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	), nil
}

func VerifyPassword(password, encoded string) (bool, error) {
	salt, hash, err := decodeArgon2ID(encoded)
	if err != nil {
		return false, err
	}
	other := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, uint32(len(hash)))
	if subtle.ConstantTimeCompare(hash, other) != 1 {
		return false, nil
	}
	return true, nil
}

func decodeArgon2ID(encoded string) (salt, hash []byte, err error) {
	if !strings.HasPrefix(encoded, "$argon2id$") {
		return nil, nil, ErrInvalidPassword
	}
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 {
		return nil, nil, ErrInvalidPassword
	}
	salt, err = base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return nil, nil, ErrInvalidPassword
	}
	hash, err = base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return nil, nil, ErrInvalidPassword
	}
	return salt, hash, nil
}

func NewToken() (string, error) {
	var b [24]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
