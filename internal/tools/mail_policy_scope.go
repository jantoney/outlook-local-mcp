package tools

import (
	"slices"

	"github.com/desek/outlook-local-mcp/internal/auth"
)

// mailPolicyScopesEqual reports whether two policies require the same ordered
// delegated OAuth scope set. It performs no mutation or authentication work.
func mailPolicyScopesEqual(left auth.MailActionPolicy, right auth.MailActionPolicy) bool {
	return slices.Equal(auth.ScopesForMailPolicy(left), auth.ScopesForMailPolicy(right))
}
