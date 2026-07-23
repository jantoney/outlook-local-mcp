package auth

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/desek/outlook-local-mcp/internal/resource"
)

// TestMigrateExplicitAccountPermissions verifies schema-one input preserves
// identity and granular mail settings while clearing implicit calendar access
// and requiring a new authentication grant.
func TestMigrateExplicitAccountPermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "accounts.json")
	legacy := map[string]any{"accounts": []any{map[string]any{
		"account_id": "11111111-1111-4111-8111-111111111111", "label": "work",
		"client_id": "client", "tenant_id": "tenant", "auth_method": "browser",
		"mail_profile": "mail_manage", "mail_policy": map[string]any{"read": true, "draft": true},
	}}}
	data, _ := json.Marshal(legacy)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := MigrateExplicitAccountPermissions(path, "test-migration-cache", dir); err != nil {
		t.Fatal(err)
	}
	accounts, err := LoadAccounts(path)
	if err != nil || len(accounts) != 1 {
		t.Fatalf("LoadAccounts() = (%v, %v)", accounts, err)
	}
	account := accounts[0]
	if account.CalendarPolicy == nil || *account.CalendarPolicy != CalendarPolicyOff {
		t.Fatalf("calendar policy = %v, want off", account.CalendarPolicy)
	}
	if !account.ReauthenticationRequired {
		t.Fatal("migration did not mark re-authentication required")
	}
	if account.MailPolicy == nil || !account.MailPolicy.Read || !account.MailPolicy.Draft {
		t.Fatalf("mail policy was not preserved: %+v", account.MailPolicy)
	}
	if account.MailProfile != "" {
		t.Fatalf("legacy mail_profile was not cleared: %q", account.MailProfile)
	}
}

// TestSetAccountSharedResourcesRejectsCrossFamilyDuplicateAlias verifies the
// persisted seam enforces the same single alias namespace as administration.
func TestSetAccountSharedResourcesRejectsCrossFamilyDuplicateAlias(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "accounts.json")
	if err := SaveAccounts(path, []AccountConfig{{Label: "work"}}); err != nil {
		t.Fatal(err)
	}
	calendar, err := resource.NewOwnerPrimaryCalendar("11111111-1111-4111-8111-111111111111", "shared", "calendar@example.com", resource.CalendarProfileRead)
	if err != nil {
		t.Fatal(err)
	}
	mailbox, err := resource.NewMailAlias("22222222-2222-4222-8222-222222222222", "shared", "mailbox@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if err := SetAccountSharedResources(path, "work", []resource.CalendarAlias{calendar}, []resource.MailAlias{mailbox}, false); err == nil || !strings.Contains(err.Error(), "duplicated") {
		t.Fatalf("SetAccountSharedResources() error = %v, want duplicate alias rejection", err)
	}
}

// TestMigrationIsIdempotent verifies a schema-two file is not reset again.
func TestMigrationIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "accounts.json")
	policy := CalendarPolicyRead
	if err := SaveAccounts(path, []AccountConfig{{Label: "work", CalendarPolicy: &policy, ReauthenticationRequired: false}}); err != nil {
		t.Fatal(err)
	}
	if err := MigrateExplicitAccountPermissions(path, "test-cache", dir); err != nil {
		t.Fatal(err)
	}
	accounts, _ := LoadAccounts(path)
	if *accounts[0].CalendarPolicy != CalendarPolicyRead || accounts[0].ReauthenticationRequired {
		t.Fatalf("schema-two state changed: %+v", accounts[0])
	}
}
