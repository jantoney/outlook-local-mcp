package auth

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/desek/outlook-local-mcp/internal/resource"
)

// TestResolveTargetProducesContextOnlyAfterExactChecks verifies unknown,
// wrong-kind, disabled, and incompatible aliases never resolve a target.
func TestResolveTargetProducesContextOnlyAfterExactChecks(t *testing.T) {
	accountID := AccountID("11111111-1111-4111-8111-111111111111")
	resourceID := resource.ResourceID("22222222-2222-4222-8222-222222222222")
	mailAlias, err := resource.NewMailAlias(resourceID, "finance", "finance@example.com")
	if err != nil {
		t.Fatalf("NewMailAlias() error = %v", err)
	}
	mailAlias.Policy = MailActionPolicy{Read: true}
	entry := &AccountEntry{AccountID: accountID, Label: "work", TokenTenantContext: TokenTenantOrganizational, MailAliases: []resource.MailAlias{mailAlias}}

	tests := []struct {
		name    string
		request TargetRequest
		context TokenTenantContext
		want    string
	}{
		{name: "unknown", request: TargetRequest{Family: resource.TargetFamilyMail, SharedResource: "missing", Capability: "read"}, context: TokenTenantOrganizational, want: "not found"},
		{name: "wrong kind", request: TargetRequest{Family: resource.TargetFamilyCalendar, SharedResource: "finance", Capability: "read"}, context: TokenTenantOrganizational, want: "mail alias"},
		{name: "disabled", request: TargetRequest{Family: resource.TargetFamilyMail, SharedResource: "finance", Capability: "send"}, context: TokenTenantOrganizational, want: "target-local policy"},
		{name: "incompatible", request: TargetRequest{Family: resource.TargetFamilyMail, SharedResource: "finance", Capability: "read"}, context: TokenTenantPersonal, want: "organizational"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			testEntry := &AccountEntry{
				AccountID: accountID, Label: "work", TokenTenantContext: test.context,
				MailAliases: []resource.MailAlias{mailAlias},
			}
			if _, resolveErr := ResolveTarget(testEntry, test.request, nil); resolveErr == nil || !strings.Contains(resolveErr.Error(), test.want) {
				t.Fatalf("ResolveTarget() error = %v, want %q", resolveErr, test.want)
			}
		})
	}
	resolved, err := ResolveTarget(entry, TargetRequest{Family: resource.TargetFamilyMail, SharedResource: "finance", Capability: "read"}, nil)
	if err != nil || resolved.Target.ResourceID != resourceID {
		t.Fatalf("ResolveTarget() = %+v, error = %v", resolved, err)
	}
}

// TestResolveTargetBindsSignedReferenceToCurrentTarget verifies a valid token
// cannot cross account, resource, kind, view, or item-kind boundaries.
func TestResolveTargetBindsSignedReferenceToCurrentTarget(t *testing.T) {
	key, err := resource.LoadOrCreateSigningKey(filepath.Join(t.TempDir(), "reference.key"))
	if err != nil {
		t.Fatalf("LoadOrCreateSigningKey() error = %v", err)
	}
	codec := resource.NewReferenceCodec(key)
	accountID := AccountID("11111111-1111-4111-8111-111111111111")
	resourceID := resource.ResourceID("22222222-2222-4222-8222-222222222222")
	alias, err := resource.NewMailAlias(resourceID, "finance", "finance@example.com")
	if err != nil {
		t.Fatalf("NewMailAlias() error = %v", err)
	}
	alias.Policy = MailActionPolicy{Read: true}
	entry := &AccountEntry{AccountID: accountID, Label: "work", TokenTenantContext: TokenTenantOrganizational, MailAliases: []resource.MailAlias{alias}}
	claims := resource.ReferenceClaims{
		AccountID: resource.AccountID(accountID), ResourceID: resourceID,
		ResourceKind: resource.ResourceKindMailbox, MailboxView: resource.MailboxViewOwner,
		ItemKind:     resource.ItemKindMessage,
		GraphIDChain: []resource.GraphID{{Kind: resource.ItemKindMessage, ID: "message/id"}},
	}
	reference, err := codec.Sign(claims)
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}
	request := TargetRequest{
		Family: resource.TargetFamilyMail, SharedResource: "finance", Capability: "read",
		Reference: reference, RequireReference: true, ItemKind: resource.ItemKindMessage,
	}
	resolved, err := ResolveTarget(entry, request, &codec)
	if err != nil || resolved.Claims == nil || resolved.Claims.GraphIDChain[0].ID != "message/id" {
		t.Fatalf("ResolveTarget() = %+v, error = %v", resolved, err)
	}
	request.ItemKind = resource.ItemKindDraft
	if _, err := ResolveTarget(entry, request, &codec); err == nil {
		t.Fatal("ResolveTarget() accepted a cross-kind reference")
	}
	entry.MailAliases = nil
	if _, err := ResolveTarget(entry, request, &codec); err == nil {
		t.Fatal("ResolveTarget() revived a removed alias")
	}
}
