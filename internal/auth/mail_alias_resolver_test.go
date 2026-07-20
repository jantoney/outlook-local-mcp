package auth

import (
	"strings"
	"testing"

	"github.com/desek/outlook-local-mcp/internal/resource"
)

// TestResolveMailAliasForCapabilityFailsClosed verifies tenant compatibility
// and exact alias-local policy deny use before routing can begin.
func TestResolveMailAliasForCapabilityFailsClosed(t *testing.T) {
	id, err := resource.NewResourceID()
	if err != nil {
		t.Fatalf("NewResourceID() error = %v", err)
	}
	alias, err := resource.NewMailAlias(id, "finance", "finance@example.com")
	if err != nil {
		t.Fatalf("NewMailAlias() error = %v", err)
	}
	alias.Policy = MailActionPolicy{Read: true}
	for _, context := range []TokenTenantContext{TokenTenantPersonal, TokenTenantUnknown} {
		entry := &AccountEntry{Label: "target", TokenTenantContext: context, MailAliases: []resource.MailAlias{alias}}
		_, resolveErr := ResolveMailAliasForCapability(entry, "finance", MailCapabilityRead)
		if resolveErr == nil || !strings.Contains(resolveErr.Error(), "organizational") {
			t.Fatalf("context %q error = %v", context, resolveErr)
		}
	}
	entry := &AccountEntry{Label: "target", TokenTenantContext: TokenTenantOrganizational, MailAliases: []resource.MailAlias{alias}}
	_, err = ResolveMailAliasForCapability(entry, "finance", MailCapabilitySend)
	if err == nil || !strings.Contains(err.Error(), "target-local policy") {
		t.Fatalf("disabled send error = %v", err)
	}
	resolved, err := ResolveMailAliasForCapability(entry, "finance", MailCapabilityRead)
	if err != nil || resolved.ResourceID != id {
		t.Fatalf("read resolution = %+v, error = %v", resolved, err)
	}
}
