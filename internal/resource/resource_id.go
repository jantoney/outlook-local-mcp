package resource

import (
	"fmt"

	"github.com/google/uuid"
)

// NewResourceID creates a cryptographically random resource identity. It returns an
// error only when secure randomness is unavailable and has no mutation side
// effects.
func NewResourceID() (ResourceID, error) {
	id, err := uuid.NewRandom()
	if err != nil {
		return "", fmt.Errorf("generate resource identity: %w", err)
	}
	return ResourceID(id.String()), nil
}

// Validate reports whether the ID is a canonical UUID. It performs no state
// changes and returns an error for empty or malformed identities.
func (id ResourceID) Validate() error {
	parsed, err := uuid.Parse(string(id))
	if err != nil || parsed.String() != string(id) {
		return fmt.Errorf("invalid resource identity %q", id)
	}
	return nil
}

// NewAccountID creates a cryptographically random signed-in account identity.
// It returns an error only when secure randomness is unavailable.
func NewAccountID() (AccountID, error) {
	id, err := uuid.NewRandom()
	if err != nil {
		return "", fmt.Errorf("generate account identity: %w", err)
	}
	return AccountID(id.String()), nil
}

// Validate reports whether the account ID is a canonical UUID. It performs no
// mutation and returns an error for empty or malformed identities.
func (id AccountID) Validate() error {
	parsed, err := uuid.Parse(string(id))
	if err != nil || parsed.String() != string(id) {
		return fmt.Errorf("invalid account identity %q", id)
	}
	return nil
}
