package auth

import (
	"fmt"

	"github.com/google/uuid"
)

// AccountID is the immutable local provenance identity of one persisted
// signed-in account instance. It is distinct from labels, UPNs, tenant IDs,
// and Microsoft identity identifiers so removing and recreating an account is
// an explicit local revocation boundary.
type AccountID string

// NewAccountID creates a cryptographically random immutable account identity.
// It returns an error only when the operating system cannot provide secure
// random bytes. The function has no side effects beyond reading randomness.
func NewAccountID() (AccountID, error) {
	id, err := uuid.NewRandom()
	if err != nil {
		return "", fmt.Errorf("generate account identity: %w", err)
	}
	return AccountID(id.String()), nil
}

// Validate verifies that the account identity is a canonical UUID. It returns
// an error for empty, malformed, or non-canonical values and has no side effects.
func (id AccountID) Validate() error {
	parsed, err := uuid.Parse(string(id))
	if err != nil || parsed.String() != string(id) {
		return fmt.Errorf("invalid account identity %q", id)
	}
	return nil
}
