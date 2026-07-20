package auth

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// TestLegacyMailProfileMigration verifies that every cumulative legacy profile
// maps only to behavior that existed before independent action policies.
func TestLegacyMailProfileMigration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		profile MailProfile
		want    MailActionPolicy
	}{
		{name: "calendar only", profile: MailProfileCalendarOnly, want: MailActionPolicy{}},
		{name: "read", profile: MailProfileRead, want: MailActionPolicy{Read: true}},
		{name: "manage", profile: MailProfileManage, want: MailActionPolicy{Read: true, Draft: true}},
		{name: "send", profile: MailProfileSend, want: MailActionPolicy{Read: true, Draft: true, Send: true}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := MailPolicyFromProfile(test.profile); got != test.want {
				t.Fatalf("MailPolicyFromProfile(%q) = %+v, want %+v", test.profile, got, test.want)
			}
		})
	}
}

// TestSetAccountMailPolicySerializesConcurrentWriters verifies policy updates
// cannot lose one another or corrupt the atomically persisted accounts file.
func TestSetAccountMailPolicySerializesConcurrentWriters(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "accounts.json")
	if err := SaveAccounts(path, []AccountConfig{
		{Label: "one", ClientID: "c", TenantID: "t", AuthMethod: "browser"},
		{Label: "two", ClientID: "c", TenantID: "t", AuthMethod: "browser"},
	}); err != nil {
		t.Fatal(err)
	}

	var wait sync.WaitGroup
	errs := make(chan error, 2)
	updates := map[string]MailActionPolicy{
		"one": {Read: true},
		"two": {Draft: true},
	}
	for label, policy := range updates {
		wait.Add(1)
		go func() {
			defer wait.Done()
			errs <- SetAccountMailPolicy(path, label, policy)
		}()
	}
	wait.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("SetAccountMailPolicy() error = %v", err)
		}
	}

	accounts, err := LoadAccounts(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, account := range accounts {
		if account.MailPolicy == nil || *account.MailPolicy != updates[account.Label] {
			t.Fatalf("account %q policy = %#v, want %+v", account.Label, account.MailPolicy, updates[account.Label])
		}
	}
}

// TestMigrateAccountMailPoliciesPersistsNewRepresentation verifies migration
// rewrites legacy records once and removes their cumulative persisted field.
func TestMigrateAccountMailPoliciesPersistsNewRepresentation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "accounts.json")
	legacy := []byte(`{"accounts":[
		{"label":"off","client_id":"c","tenant_id":"t","auth_method":"browser","upn":"","mail_profile":"calendar_only"},
		{"label":"read","client_id":"c","tenant_id":"t","auth_method":"browser","upn":"","mail_profile":"mail_read"},
		{"label":"manage","client_id":"c","tenant_id":"t","auth_method":"browser","upn":"","mail_profile":"mail_manage"},
		{"label":"send","client_id":"c","tenant_id":"t","auth_method":"browser","upn":"","mail_profile":"mail_send"}
	]}`)
	if err := os.WriteFile(path, legacy, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := MigrateAccountMailPolicies(path, MailProfileCalendarOnly); err != nil {
		t.Fatalf("MigrateAccountMailPolicies() error = %v", err)
	}
	accounts, err := LoadAccounts(path)
	if err != nil {
		t.Fatal(err)
	}
	want := []MailActionPolicy{
		{},
		{Read: true},
		{Read: true, Draft: true},
		{Read: true, Draft: true, Send: true},
	}
	for index := range accounts {
		if accounts[index].MailPolicy == nil || *accounts[index].MailPolicy != want[index] {
			t.Fatalf("account %q policy = %#v, want %+v", accounts[index].Label, accounts[index].MailPolicy, want[index])
		}
		if accounts[index].MailProfile != "" {
			t.Fatalf("account %q retained legacy mail_profile %q", accounts[index].Label, accounts[index].MailProfile)
		}
	}
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := MigrateAccountMailPolicies(path, MailProfileCalendarOnly); err != nil {
		t.Fatalf("second MigrateAccountMailPolicies() error = %v", err)
	}
	second, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(second) != string(first) {
		t.Fatal("idempotent migration rewrote the accounts file")
	}
}
