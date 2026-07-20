package auth

import (
	"path/filepath"
	"sync"
	"testing"

	"github.com/desek/outlook-local-mcp/internal/resource"
)

// TestSetAccountCalendarAliasesPersistsCompleteIdentity verifies aliases
// survive reload with immutable routing and target-policy fields intact.
func TestSetAccountCalendarAliasesPersistsCompleteIdentity(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "accounts.json")
	if err := SaveAccounts(path, []AccountConfig{{Label: "work"}}); err != nil {
		t.Fatalf("SaveAccounts() error = %v", err)
	}
	id, err := resource.NewResourceID()
	if err != nil {
		t.Fatalf("NewResourceID() error = %v", err)
	}
	alias, err := resource.NewMountedCalendar(
		id, "team", "owner@example.com", "mounted-id", resource.CalendarProfileRead,
	)
	if err != nil {
		t.Fatalf("NewMountedCalendar() error = %v", err)
	}
	if err := SetAccountCalendarAliases(path, "work", []resource.CalendarAlias{alias}); err != nil {
		t.Fatalf("SetAccountCalendarAliases() error = %v", err)
	}
	got, err := AccountCalendarAliases(path, "work")
	if err != nil {
		t.Fatalf("AccountCalendarAliases() error = %v", err)
	}
	if len(got) != 1 || got[0] != alias {
		t.Fatalf("AccountCalendarAliases() = %+v, want %+v", got, alias)
	}
}

// TestSetAccountCalendarAliasesSerializesWithMailPolicy verifies calendar and
// mail policy updates share one read-modify-write lock and cannot lose either
// account's persisted change.
func TestSetAccountCalendarAliasesSerializesWithMailPolicy(t *testing.T) {
	path := filepath.Join(t.TempDir(), "accounts.json")
	if err := SaveAccounts(path, []AccountConfig{{Label: "calendar"}, {Label: "mail"}}); err != nil {
		t.Fatalf("SaveAccounts() error = %v", err)
	}
	id, err := resource.NewResourceID()
	if err != nil {
		t.Fatalf("NewResourceID() error = %v", err)
	}
	alias, err := resource.NewMountedCalendar(id, "team", "owner@example.com", "mounted-id", resource.CalendarProfileRead)
	if err != nil {
		t.Fatalf("NewMountedCalendar() error = %v", err)
	}

	var wait sync.WaitGroup
	errs := make(chan error, 2)
	wait.Add(2)
	go func() {
		defer wait.Done()
		errs <- SetAccountCalendarAliases(path, "calendar", []resource.CalendarAlias{alias})
	}()
	go func() {
		defer wait.Done()
		errs <- SetAccountMailPolicy(path, "mail", MailActionPolicy{Send: true})
	}()
	wait.Wait()
	close(errs)
	for updateErr := range errs {
		if updateErr != nil {
			t.Fatalf("concurrent account update error = %v", updateErr)
		}
	}

	accounts, err := LoadAccounts(path)
	if err != nil {
		t.Fatalf("LoadAccounts() error = %v", err)
	}
	if accounts[0].CalendarAliases == nil || len(*accounts[0].CalendarAliases) != 1 {
		t.Fatalf("calendar aliases = %+v, want one persisted alias", accounts[0].CalendarAliases)
	}
	if accounts[1].MailPolicy == nil || !accounts[1].MailPolicy.Send {
		t.Fatalf("mail policy = %+v, want send enabled", accounts[1].MailPolicy)
	}
}

// TestLegacyAccountCalendarAliasesDefaultOff verifies records predating shared
// resources load with no aliases and no shared scope contribution.
func TestLegacyAccountCalendarAliasesDefaultOff(t *testing.T) {
	t.Parallel()

	account := AccountConfig{Label: "legacy"}
	if account.CalendarAliases != nil {
		t.Fatal("legacy CalendarAliases is non-nil")
	}
	for _, scope := range ScopesForAccountConfig(account) {
		if scope == calendarReadSharedScope || scope == calendarReadWriteSharedScope {
			t.Fatalf("legacy account unexpectedly requests shared scope %q", scope)
		}
	}
}
