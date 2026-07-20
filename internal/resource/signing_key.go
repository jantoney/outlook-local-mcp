package resource

import (
	"crypto/rand"
	"fmt"
)

// SigningKeySize is the required byte length of an HMAC-SHA-256 signing key.
const SigningKeySize int64 = 32

// SigningKey is the local secret authority used to sign resource references.
// Its bytes remain unexported so callers cannot accidentally serialize or log
// key material.
type SigningKey struct {
	bytes [SigningKeySize]byte
}

// newSigningKey creates a signing key from operating-system cryptographic
// randomness. It returns an error when secure randomness is unavailable and
// has no other side effects.
func newSigningKey() (SigningKey, error) {
	var key SigningKey
	if _, err := rand.Read(key.bytes[:]); err != nil {
		return SigningKey{}, fmt.Errorf("generate signing key: %w", err)
	}
	return key, nil
}
