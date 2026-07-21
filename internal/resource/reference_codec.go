package resource

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const (
	// ReferenceVersion is the on-wire version prefix for signed resource
	// references produced by this implementation.
	ReferenceVersion   = "v1"
	maxReferenceLength = 16 * 1024
)

// ErrInvalidReference is returned when a resource reference is malformed,
// unsupported, corrupted, or signed by another key.
var ErrInvalidReference = errors.New("invalid resource reference")

// ReferenceCodec signs and verifies self-contained resource provenance using
// one local signing authority.
type ReferenceCodec struct {
	key SigningKey
}

// KeyedFingerprint returns a domain-separated HMAC fingerprint suitable for
// audit correlation without storing sensitive review evidence. The result is
// truncated to 128 bits and cannot be used to authorize a resource.
func (c ReferenceCodec) KeyedFingerprint(label, value string) string {
	mac := hmac.New(sha256.New, c.key.bytes[:])
	_, _ = mac.Write([]byte("audit:" + label + "\x00" + value))
	return hex.EncodeToString(mac.Sum(nil)[:16])
}

// NewReferenceCodec creates a codec bound to key. The returned codec performs
// no I/O and contains no authorization state beyond the signing authority.
func NewReferenceCodec(key SigningKey) ReferenceCodec {
	return ReferenceCodec{key: key}
}

// Sign validates and signs claims as a versioned, URL-safe opaque reference.
// It returns the reference or an error for incomplete or inconsistent claims.
// The method performs no persistence or Graph calls.
func (c ReferenceCodec) Sign(claims ReferenceClaims) (string, error) {
	if err := claims.validate(); err != nil {
		return "", fmt.Errorf("sign resource reference: %w", err)
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("marshal resource reference: %w", err)
	}
	encodedPayload := base64.RawURLEncoding.EncodeToString(payload)
	unsigned := ReferenceVersion + "." + encodedPayload
	signature := c.signature(unsigned)
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

// Verify authenticates and decodes a versioned resource reference. It returns
// the original immutable provenance claims or ErrInvalidReference. Successful
// verification does not authorize the referenced account, resource, or item.
func (c ReferenceCodec) Verify(reference string) (ReferenceClaims, error) {
	if len(reference) == 0 || len(reference) > maxReferenceLength {
		return ReferenceClaims{}, ErrInvalidReference
	}
	parts := strings.Split(reference, ".")
	if len(parts) != 3 || parts[0] != ReferenceVersion {
		return ReferenceClaims{}, ErrInvalidReference
	}
	unsigned := parts[0] + "." + parts[1]
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || !hmac.Equal(signature, c.signature(unsigned)) {
		return ReferenceClaims{}, ErrInvalidReference
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ReferenceClaims{}, ErrInvalidReference
	}
	var claims ReferenceClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return ReferenceClaims{}, ErrInvalidReference
	}
	if err := claims.validate(); err != nil {
		return ReferenceClaims{}, fmt.Errorf("%w: %v", ErrInvalidReference, err)
	}
	return claims, nil
}

// signature returns the HMAC-SHA-256 authentication tag for unsigned using the
// codec's local key. It has no side effects.
func (c ReferenceCodec) signature(unsigned string) []byte {
	mac := hmac.New(sha256.New, c.key.bytes[:])
	_, _ = mac.Write([]byte(unsigned))
	return mac.Sum(nil)
}
