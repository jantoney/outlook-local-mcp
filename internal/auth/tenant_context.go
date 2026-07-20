package auth

import (
	"regexp"
	"strings"
)

// MicrosoftConsumerTenantID is the fixed tenant identifier used when a token
// is issued directly in the Microsoft consumer identity tenant.
const MicrosoftConsumerTenantID = "9188040d-6c67-4c5b-b112-36a304b66dad"

var tenantGUIDPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// TokenTenantContext describes the directory context that issued a validated
// token. It deliberately does not claim to describe a guest user's home
// identity.
type TokenTenantContext string

const (
	// TokenTenantUnknown means no validated tenant GUID was available.
	TokenTenantUnknown TokenTenantContext = "unknown"
	// TokenTenantPersonal means the token was issued by Microsoft's consumer tenant.
	TokenTenantPersonal TokenTenantContext = "personal"
	// TokenTenantOrganizational means the token was issued by another tenant GUID.
	TokenTenantOrganizational TokenTenantContext = "organizational"
)

// NormalizeTokenTenantContext returns context when it is a recognized value
// and unknown otherwise. It keeps zero-value and future malformed runtime
// entries fail-closed without consulting presentation metadata.
func NormalizeTokenTenantContext(context TokenTenantContext) TokenTenantContext {
	switch context {
	case TokenTenantPersonal, TokenTenantOrganizational:
		return context
	default:
		return TokenTenantUnknown
	}
}

// ClassifyTokenTenantContext classifies a validated token tenant identifier.
// Authority aliases, email addresses, tenant names, opaque account IDs, and
// malformed values return unknown so callers cannot infer identity origin from
// presentation metadata.
func ClassifyTokenTenantContext(tenantID string) TokenTenantContext {
	if !validTenantGUID(tenantID) {
		return TokenTenantUnknown
	}
	if strings.EqualFold(tenantID, MicrosoftConsumerTenantID) {
		return TokenTenantPersonal
	}
	return TokenTenantOrganizational
}

// validTenantGUID reports whether tenantID has the canonical GUID shape used
// by Microsoft identity token tenant claims. It performs no mutation.
func validTenantGUID(tenantID string) bool {
	return tenantGUIDPattern.MatchString(tenantID)
}
