package auth

import (
	"github.com/desek/outlook-local-mcp/internal/resource"
)

// AccountID is the immutable local provenance identity of one persisted
// signed-in account instance. It is distinct from labels, UPNs, tenant IDs,
// and Microsoft identity identifiers so removing and recreating an account is
// an explicit local revocation boundary.
type AccountID = resource.AccountID

// NewAccountID creates a cryptographically random immutable account identity.
// It returns an error only when the operating system cannot provide secure
// random bytes. The function has no side effects beyond reading randomness.
func NewAccountID() (AccountID, error) {
	return resource.NewAccountID()
}
