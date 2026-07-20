package auth

import (
	"slices"
	"testing"

	"github.com/desek/outlook-local-mcp/internal/resource"
)

// TestScopesForMailAliases verifies exact shared-resource scopes and keeps own
// and shared policies independent despite their account-wide OAuth union.
func TestScopesForMailAliases(t *testing.T) {
	id, err := resource.NewResourceID()
	if err != nil {
		t.Fatalf("NewResourceID() error = %v", err)
	}
	alias, err := resource.NewMailAlias(id, "finance", "finance@example.com")
	if err != nil {
		t.Fatalf("NewMailAlias() error = %v", err)
	}
	if scopes := ScopesForMailAliases([]resource.MailAlias{alias}); len(scopes) != 0 {
		t.Fatalf("default-off shared-mail scopes = %v, want none", scopes)
	}
	alias.Policy = MailActionPolicy{Send: true}
	scopes := ScopesForMailAliases([]resource.MailAlias{alias})
	for _, want := range []string{"Mail.ReadWrite.Shared", "Mail.Send.Shared"} {
		if !slices.Contains(scopes, want) {
			t.Fatalf("send scopes = %v, want %s", scopes, want)
		}
	}
	if slices.Contains(scopes, "Mail.Send") {
		t.Fatalf("shared policy leaked own-mail scope: %v", scopes)
	}
}
