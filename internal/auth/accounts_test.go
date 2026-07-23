package auth

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSaveAccountsRenameFailurePreservesOriginal verifies the atomic writer's
// failure path without relying on platform-specific directory permissions.
func TestSaveAccountsRenameFailurePreservesOriginal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "accounts.json")
	original := []AccountConfig{{Label: "alpha"}, {Label: "beta"}}
	if err := SaveAccounts(path, original); err != nil {
		t.Fatal(err)
	}
	wantErr := errors.New("injected replacement failure")
	err := saveAccountsWithRename(path, []AccountConfig{{Label: "beta"}}, func(string, string) error { return wantErr })
	if !errors.Is(err, wantErr) {
		t.Fatalf("saveAccountsWithRename() error = %v, want injected failure", err)
	}
	accounts, loadErr := LoadAccounts(path)
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if len(accounts) != 2 || accounts[0].Label != "alpha" || accounts[1].Label != "beta" {
		t.Fatalf("original accounts changed after failed replacement: %+v", accounts)
	}
	leftovers, globErr := filepath.Glob(filepath.Join(filepath.Dir(path), "accounts-*.json.tmp"))
	if globErr != nil {
		t.Fatal(globErr)
	}
	if len(leftovers) != 0 {
		t.Fatalf("temporary files left after failed replacement: %v", leftovers)
	}
}

// TestSaveAndLoadAccounts verifies that account configurations survive a
// round-trip through SaveAccounts and LoadAccounts.
func TestSaveAndLoadAccounts(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "accounts.json")

	want := []AccountConfig{
		{Label: "work", ClientID: "aaaa", TenantID: "tenant-a", AuthMethod: "browser"},
		{Label: "personal", ClientID: "bbbb", TenantID: "common", AuthMethod: "device_code"},
	}

	if err := SaveAccounts(path, want); err != nil {
		t.Fatalf("SaveAccounts: %v", err)
	}

	got, err := LoadAccounts(path)
	if err != nil {
		t.Fatalf("LoadAccounts: %v", err)
	}

	if len(got) != len(want) {
		t.Fatalf("LoadAccounts returned %d accounts, want %d", len(got), len(want))
	}

	for i := range want {
		if got[i] != want[i] {
			t.Errorf("account[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// TestAccountProfileRoundTrip verifies that account-specific risk profiles
// survive persistence without changing their stable string representation.
func TestAccountProfileRoundTrip(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "accounts.json")
	want := []AccountConfig{{
		Label: "operations", ClientID: "client", TenantID: "common",
		AuthMethod: "device_code", MailProfile: "mail_send",
	}}
	if err := SaveAccounts(path, want); err != nil {
		t.Fatalf("SaveAccounts() error = %v", err)
	}
	got, err := LoadAccounts(path)
	if err != nil {
		t.Fatalf("LoadAccounts() error = %v", err)
	}
	if len(got) != 1 || got[0].MailProfile != "mail_send" {
		t.Fatalf("LoadAccounts() = %+v, want mail_send profile", got)
	}
}

// TestLoadAccounts_FileNotExist verifies that LoadAccounts returns an empty
// slice with no error when the accounts file does not exist.
func TestLoadAccounts_FileNotExist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nonexistent", "accounts.json")

	accounts, err := LoadAccounts(path)
	if err != nil {
		t.Fatalf("LoadAccounts: %v", err)
	}

	if len(accounts) != 0 {
		t.Errorf("LoadAccounts returned %d accounts, want 0", len(accounts))
	}
}

// TestAddAccountConfig verifies that AddAccountConfig appends a new account
// to the existing accounts file.
func TestAddAccountConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "accounts.json")

	initial := []AccountConfig{
		{Label: "work", ClientID: "aaaa", TenantID: "tenant-a", AuthMethod: "browser"},
	}
	if err := SaveAccounts(path, initial); err != nil {
		t.Fatalf("SaveAccounts: %v", err)
	}

	newConfig := AccountConfig{
		Label:      "personal",
		ClientID:   "bbbb",
		TenantID:   "common",
		AuthMethod: "device_code",
	}
	if err := AddAccountConfig(path, newConfig); err != nil {
		t.Fatalf("AddAccountConfig: %v", err)
	}

	got, err := LoadAccounts(path)
	if err != nil {
		t.Fatalf("LoadAccounts: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("LoadAccounts returned %d accounts, want 2", len(got))
	}

	if got[0].Label != "work" {
		t.Errorf("account[0].Label = %q, want %q", got[0].Label, "work")
	}
	if got[1].Label != "personal" {
		t.Errorf("account[1].Label = %q, want %q", got[1].Label, "personal")
	}
}

// TestUpsertAccountConfigAddsImplicitAccount verifies a runtime-only default
// account becomes a complete persisted record when its profile is changed.
func TestUpsertAccountConfigAddsImplicitAccount(t *testing.T) {
	path := filepath.Join(t.TempDir(), "accounts.json")
	want := AccountConfig{Label: "default", ClientID: "client", TenantID: "common", AuthMethod: "device_code", UPN: "me@example.com", MailProfile: "mail_read"}
	if err := UpsertAccountConfig(path, want); err != nil {
		t.Fatal(err)
	}
	accounts, err := LoadAccounts(path)
	if err != nil || len(accounts) != 1 {
		t.Fatalf("LoadAccounts() = (%+v, %v), want one account", accounts, err)
	}
	if accounts[0].AccountID == "" {
		t.Fatal("UpsertAccountConfig() did not generate an immutable account identity")
	}
	accounts[0].AccountID = ""
	if accounts[0] != want {
		t.Fatalf("LoadAccounts() = %+v, want %+v", accounts[0], want)
	}
}

// TestRemoveAccountConfig verifies that RemoveAccountConfig removes the
// account with the given label from the accounts file.
func TestRemoveAccountConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "accounts.json")

	initial := []AccountConfig{
		{Label: "work", ClientID: "aaaa", TenantID: "tenant-a", AuthMethod: "browser"},
		{Label: "personal", ClientID: "bbbb", TenantID: "common", AuthMethod: "device_code"},
	}
	if err := SaveAccounts(path, initial); err != nil {
		t.Fatalf("SaveAccounts: %v", err)
	}

	if err := RemoveAccountConfig(path, "work"); err != nil {
		t.Fatalf("RemoveAccountConfig: %v", err)
	}

	got, err := LoadAccounts(path)
	if err != nil {
		t.Fatalf("LoadAccounts: %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("LoadAccounts returned %d accounts, want 1", len(got))
	}

	if got[0].Label != "personal" {
		t.Errorf("remaining account Label = %q, want %q", got[0].Label, "personal")
	}
}

// TestUpdateAccountUPN_Success verifies that UpdateAccountUPN persists the
// supplied UPN onto the matching account entry in accounts.json.
func TestUpdateAccountUPN_Success(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "accounts.json")

	initial := []AccountConfig{
		{Label: "work", ClientID: "aaaa", TenantID: "tenant-a", AuthMethod: "browser"},
	}
	if err := SaveAccounts(path, initial); err != nil {
		t.Fatalf("SaveAccounts: %v", err)
	}

	if err := UpdateAccountUPN(path, "work", "alice@contoso.com"); err != nil {
		t.Fatalf("UpdateAccountUPN: %v", err)
	}

	got, err := LoadAccounts(path)
	if err != nil {
		t.Fatalf("LoadAccounts: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("LoadAccounts returned %d accounts, want 1", len(got))
	}
	if got[0].UPN != "alice@contoso.com" {
		t.Errorf("UPN = %q, want %q", got[0].UPN, "alice@contoso.com")
	}
}

// TestUpdateAccountUPN_NotFound verifies that UpdateAccountUPN is a silent
// no-op when the label is not present in accounts.json, leaving the file
// unchanged.
func TestUpdateAccountUPN_NotFound(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "accounts.json")

	initial := []AccountConfig{
		{Label: "work", ClientID: "aaaa", TenantID: "tenant-a", AuthMethod: "browser"},
	}
	if err := SaveAccounts(path, initial); err != nil {
		t.Fatalf("SaveAccounts: %v", err)
	}

	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile before: %v", err)
	}

	if err := UpdateAccountUPN(path, "ghost", "nobody@example.com"); err != nil {
		t.Fatalf("UpdateAccountUPN: %v", err)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile after: %v", err)
	}

	if string(before) != string(after) {
		t.Error("file content changed when label was not found")
	}
}

// TestFindByIdentity_Match verifies that FindByIdentity returns the first entry
// whose ClientID and TenantID both match the supplied arguments.
func TestFindByIdentity_Match(t *testing.T) {
	accounts := []AccountConfig{
		{Label: "other", ClientID: "aaa", TenantID: "tenant-x", AuthMethod: "browser"},
		{Label: "work", ClientID: "cid-1", TenantID: "tid-1", AuthMethod: "browser"},
	}

	got, ok := FindByIdentity(accounts, "cid-1", "tid-1")
	if !ok {
		t.Fatal("FindByIdentity returned false, want true")
	}
	if got.Label != "work" {
		t.Errorf("Label = %q, want %q", got.Label, "work")
	}
}

// TestFindByIdentity_NoMatch verifies that FindByIdentity returns (zero, false)
// when no entry matches the supplied clientID and tenantID.
func TestFindByIdentity_NoMatch(t *testing.T) {
	accounts := []AccountConfig{
		{Label: "work", ClientID: "cid-1", TenantID: "tid-1", AuthMethod: "browser"},
	}

	got, ok := FindByIdentity(accounts, "cid-99", "tid-99")
	if ok {
		t.Fatalf("FindByIdentity returned true for non-matching identity, entry = %+v", got)
	}
	if got != (AccountConfig{}) {
		t.Errorf("FindByIdentity returned non-zero entry on no-match: %+v", got)
	}
}

// TestFindByIdentity_EmptyArgs verifies that FindByIdentity returns (zero, false)
// when either clientID or tenantID is empty, preventing spurious matches.
func TestFindByIdentity_EmptyArgs(t *testing.T) {
	accounts := []AccountConfig{
		{Label: "work", ClientID: "cid-1", TenantID: "tid-1", AuthMethod: "browser"},
	}

	tests := []struct {
		name     string
		clientID string
		tenantID string
	}{
		{"empty clientID", "", "tid-1"},
		{"empty tenantID", "cid-1", ""},
		{"both empty", "", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := FindByIdentity(accounts, tc.clientID, tc.tenantID)
			if ok {
				t.Fatalf("FindByIdentity returned true for empty arg case %q, entry = %+v", tc.name, got)
			}
			if got != (AccountConfig{}) {
				t.Errorf("FindByIdentity returned non-zero entry: %+v", got)
			}
		})
	}
}

// TestRemoveAccountConfig_NotFound verifies that RemoveAccountConfig returns
// no error and leaves the file unchanged when the label is not found.
func TestRemoveAccountConfig_NotFound(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "accounts.json")

	initial := []AccountConfig{
		{Label: "work", ClientID: "aaaa", TenantID: "tenant-a", AuthMethod: "browser"},
	}
	if err := SaveAccounts(path, initial); err != nil {
		t.Fatalf("SaveAccounts: %v", err)
	}

	// Capture file content before removal attempt.
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile before: %v", err)
	}

	if err := RemoveAccountConfig(path, "ghost"); err != nil {
		t.Fatalf("RemoveAccountConfig: %v", err)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile after: %v", err)
	}

	if string(before) != string(after) {
		t.Error("file content changed after removing non-existent label")
	}
}

// TestMigrateAccountIDsPersistsLegacyIdentity verifies that migration assigns
// one immutable identity to a legacy account and preserves it across reloads.
func TestMigrateAccountIDsPersistsLegacyIdentity(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "accounts.json")
	legacy := []AccountConfig{{
		Label: "work", ClientID: "client", TenantID: "tenant", AuthMethod: "browser",
	}}
	if err := SaveAccounts(path, legacy); err != nil {
		t.Fatalf("SaveAccounts() error = %v", err)
	}

	if err := MigrateAccountIDs(path); err != nil {
		t.Fatalf("MigrateAccountIDs() error = %v", err)
	}
	first, err := LoadAccounts(path)
	if err != nil {
		t.Fatalf("LoadAccounts() error = %v", err)
	}
	if len(first) != 1 || first[0].AccountID == "" {
		t.Fatalf("LoadAccounts() = %+v, want one migrated account identity", first)
	}

	if err := MigrateAccountIDs(path); err != nil {
		t.Fatalf("second MigrateAccountIDs() error = %v", err)
	}
	second, err := LoadAccounts(path)
	if err != nil {
		t.Fatalf("second LoadAccounts() error = %v", err)
	}
	if second[0].AccountID != first[0].AccountID {
		t.Fatalf("account identity changed across restart: got %q, want %q", second[0].AccountID, first[0].AccountID)
	}
}

// TestLegacyAccountsDefaultSharedOff verifies deterministic legacy data gains
// only immutable provenance and does not opt into shared access or scopes.
func TestLegacyAccountsDefaultSharedOff(t *testing.T) {
	t.Parallel()

	fixture, err := os.ReadFile(filepath.Join("testdata", "legacy_accounts.json"))
	if err != nil {
		t.Fatalf("ReadFile(fixture) error = %v", err)
	}
	path := filepath.Join(t.TempDir(), "accounts.json")
	if err := os.WriteFile(path, fixture, 0o600); err != nil {
		t.Fatalf("WriteFile(accounts) error = %v", err)
	}

	if err := MigrateAccountIDs(path); err != nil {
		t.Fatalf("MigrateAccountIDs() error = %v", err)
	}
	accounts, err := LoadAccounts(path)
	if err != nil {
		t.Fatalf("LoadAccounts() error = %v", err)
	}
	if len(accounts) != 1 {
		t.Fatalf("LoadAccounts() count = %d, want 1", len(accounts))
	}
	got := accounts[0]
	if got.Label != "legacy-work" || got.ClientID != "legacy-client" ||
		got.TenantID != "legacy-tenant" || got.AuthMethod != "browser" ||
		got.UPN != "legacy@example.com" || got.MailProfile != "" {
		t.Fatalf("migration changed legacy behavior: %+v", got)
	}
	if got.AccountID == "" {
		t.Fatal("migration did not assign account identity")
	}
	for _, scope := range ScopesForProfile(MailProfileCalendarOnly) {
		if strings.HasSuffix(scope, ".Shared") {
			t.Fatalf("legacy migration enabled shared scope %q", scope)
		}
	}
	migrated, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(migrated) error = %v", err)
	}
	if strings.Contains(strings.ToLower(string(migrated)), "shared") {
		t.Fatalf("legacy migration enabled shared access: %s", migrated)
	}
}

// TestRemoveAndRecreateAccountGetsNewIdentity verifies that account removal is
// a provenance revocation boundary even when the same label is deliberately reused.
func TestRemoveAndRecreateAccountGetsNewIdentity(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "accounts.json")
	config := AccountConfig{
		Label: "work", ClientID: "client", TenantID: "tenant", AuthMethod: "browser",
	}
	if err := AddAccountConfig(path, config); err != nil {
		t.Fatalf("AddAccountConfig() error = %v", err)
	}
	first, err := LoadAccounts(path)
	if err != nil {
		t.Fatalf("LoadAccounts() error = %v", err)
	}
	if len(first) != 1 || first[0].AccountID == "" {
		t.Fatalf("first account = %+v, want generated identity", first)
	}

	if err := RemoveAccountConfig(path, "work"); err != nil {
		t.Fatalf("RemoveAccountConfig() error = %v", err)
	}
	if err := AddAccountConfig(path, config); err != nil {
		t.Fatalf("second AddAccountConfig() error = %v", err)
	}
	second, err := LoadAccounts(path)
	if err != nil {
		t.Fatalf("second LoadAccounts() error = %v", err)
	}
	if second[0].AccountID == first[0].AccountID {
		t.Fatalf("recreated account reused identity %q", second[0].AccountID)
	}
}

// TestUpsertAccountConfigPreservesIdentity verifies that presentation and
// profile updates cannot silently replace an existing account identity.
func TestUpsertAccountConfigPreservesIdentity(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "accounts.json")
	if err := AddAccountConfig(path, AccountConfig{
		Label: "work", ClientID: "client", TenantID: "tenant", AuthMethod: "browser",
	}); err != nil {
		t.Fatalf("AddAccountConfig() error = %v", err)
	}
	before, err := LoadAccounts(path)
	if err != nil {
		t.Fatalf("LoadAccounts() error = %v", err)
	}

	if err := UpsertAccountConfig(path, AccountConfig{
		Label: "work", ClientID: "client", TenantID: "tenant", AuthMethod: "browser",
		UPN: "alice@example.com", MailProfile: "mail_read",
	}); err != nil {
		t.Fatalf("UpsertAccountConfig() error = %v", err)
	}
	after, err := LoadAccounts(path)
	if err != nil {
		t.Fatalf("second LoadAccounts() error = %v", err)
	}
	if after[0].AccountID != before[0].AccountID {
		t.Fatalf("UpsertAccountConfig() identity = %q, want %q", after[0].AccountID, before[0].AccountID)
	}
	if after[0].UPN != "alice@example.com" || after[0].MailProfile != "mail_read" {
		t.Fatalf("UpsertAccountConfig() = %+v, want updated presentation and profile", after[0])
	}
}

// TestUpsertLegacyAccountAssignsIdentity verifies that a direct update cannot
// leave a pre-migration account without immutable provenance.
func TestUpsertLegacyAccountAssignsIdentity(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "accounts.json")
	legacy := AccountConfig{
		Label: "work", ClientID: "client", TenantID: "tenant", AuthMethod: "browser",
	}
	if err := SaveAccounts(path, []AccountConfig{legacy}); err != nil {
		t.Fatalf("SaveAccounts() error = %v", err)
	}
	legacy.UPN = "alice@example.com"
	if err := UpsertAccountConfig(path, legacy); err != nil {
		t.Fatalf("UpsertAccountConfig() error = %v", err)
	}
	accounts, err := LoadAccounts(path)
	if err != nil {
		t.Fatalf("LoadAccounts() error = %v", err)
	}
	if accounts[0].AccountID == "" {
		t.Fatal("legacy upsert preserved an empty account identity")
	}
}
