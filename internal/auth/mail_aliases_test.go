package auth

import (
	"path/filepath"
	"testing"

	"github.com/desek/outlook-local-mcp/internal/resource"
)

// TestSetAccountMailAliasesPersistsSeparateFamily verifies shared mail remains
// distinct from a same-owner calendar alias and survives a persistence reload.
func TestSetAccountMailAliasesPersistsSeparateFamily(t *testing.T) {
	path := filepath.Join(t.TempDir(), "accounts.json")
	if err := SaveAccounts(path, []AccountConfig{{Label: "work"}}); err != nil {
		t.Fatalf("SaveAccounts() error = %v", err)
	}
	id, err := resource.NewResourceID()
	if err != nil {
		t.Fatalf("NewResourceID() error = %v", err)
	}
	alias, err := resource.NewMailAlias(id, "finance-mail", "finance@example.com")
	if err != nil {
		t.Fatalf("NewMailAlias() error = %v", err)
	}
	if err := SetAccountMailAliases(path, "work", []resource.MailAlias{alias}); err != nil {
		t.Fatalf("SetAccountMailAliases() error = %v", err)
	}
	got, err := AccountMailAliases(path, "work")
	if err != nil || len(got) != 1 || got[0] != alias {
		t.Fatalf("AccountMailAliases() = %+v, error = %v", got, err)
	}
	accounts, err := LoadAccounts(path)
	if err != nil {
		t.Fatalf("LoadAccounts() error = %v", err)
	}
	if accounts[0].CalendarAliases != nil {
		t.Fatalf("mail alias unexpectedly populated calendar aliases: %+v", accounts[0].CalendarAliases)
	}
}
