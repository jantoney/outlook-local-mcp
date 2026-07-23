package accountadmin

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/desek/outlook-local-mcp/internal/auth"
	"github.com/desek/outlook-local-mcp/internal/config"
	"github.com/desek/outlook-local-mcp/internal/resource"
	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"
)

// newPermissionTestModule creates one persisted and registered account for
// permission-transition tests.
func newPermissionTestModule(t *testing.T, calendar auth.CalendarPolicy, mail auth.MailActionPolicy) (*Module, *auth.AccountRegistry) {
	t.Helper()
	dir := t.TempDir()
	cfg := config.Config{AccountsPath: filepath.Join(dir, "accounts.json"), AuthRecordPath: filepath.Join(dir, "auth.json")}
	accountID := auth.AccountID("11111111-1111-4111-8111-111111111111")
	if err := auth.SaveAccounts(cfg.AccountsPath, []auth.AccountConfig{{
		AccountID: accountID, Label: "work", ClientID: "client", TenantID: "tenant", AuthMethod: "browser",
		CalendarPolicy: &calendar, MailPolicy: &mail,
	}}); err != nil {
		t.Fatal(err)
	}
	registry := auth.NewAccountRegistry()
	if err := registry.Add(&auth.AccountEntry{
		AccountID: accountID, Label: "work", ClientID: "client", TenantID: "tenant", AuthMethod: "browser",
		CalendarPolicy: calendar, MailPolicy: mail, Authenticated: true,
		Client: &msgraphsdk.GraphServiceClient{}, Scopes: auth.RequiredScopes(calendar, mail, nil, nil),
	}); err != nil {
		t.Fatal(err)
	}
	return New(context.Background(), cfg, registry), registry
}

// TestSharedAliasesUseOneNamespace verifies a calendar and mailbox cannot share
// one selector within an account.
func TestSharedAliasesUseOneNamespace(t *testing.T) {
	module, _ := newPermissionTestModule(t, auth.CalendarPolicyOff, auth.MailActionPolicy{})
	if _, err := module.AddMailbox("work", "finance", "finance@example.com"); err != nil {
		t.Fatal(err)
	}
	if _, err := module.AddCalendar("work", "finance", "owner@example.com", resource.CalendarKindMounted, "calendar-id", resource.CalendarProfileOff); err == nil {
		t.Fatal("AddCalendar() accepted a duplicate cross-family alias")
	}
}

// TestSetPermissionsSameScopeAppliesImmediately verifies local policy changes
// sharing the same OAuth scope keep the active session connected.
func TestSetPermissionsSameScopeAppliesImmediately(t *testing.T) {
	oldMail := auth.MailActionPolicy{Draft: true}
	module, registry := newPermissionTestModule(t, auth.CalendarPolicyOff, oldMail)
	updated, err := module.SetPermissions("work", PermissionUpdate{Calendar: auth.CalendarPolicyOff, Mail: auth.MailActionPolicy{Move: true}})
	if err != nil {
		t.Fatal(err)
	}
	entry, _ := registry.Get("work")
	if !updated.Authenticated || updated.ReauthenticationRequired || entry.Client == nil {
		t.Fatalf("same-scope update disconnected account: %+v", updated)
	}
}

// TestSetPermissionsScopeChangeRequiresReauth verifies a broader calendar
// scope saves immediately but disconnects and fails closed until authentication.
func TestSetPermissionsScopeChangeRequiresReauth(t *testing.T) {
	module, registry := newPermissionTestModule(t, auth.CalendarPolicyOff, auth.MailActionPolicy{})
	updated, err := module.SetPermissions("work", PermissionUpdate{Calendar: auth.CalendarPolicyRead})
	if err != nil {
		t.Fatal(err)
	}
	entry, _ := registry.Get("work")
	if updated.Authenticated || !updated.ReauthenticationRequired || entry.Client != nil {
		t.Fatalf("scope-changing update did not fail closed: %+v", updated)
	}
	accounts, _ := auth.LoadAccounts(module.cfg.AccountsPath)
	if !accounts[0].ReauthenticationRequired || *accounts[0].CalendarPolicy != auth.CalendarPolicyRead {
		t.Fatalf("persisted state incorrect: %+v", accounts[0])
	}
}

// TestCreateAccountDefaultsOptionalPermissionsOff verifies account creation is
// disconnected and requests only required identity scope by default.
func TestCreateAccountDefaultsOptionalPermissionsOff(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Config{ClientID: "client", TenantID: "tenant", AuthMethod: "browser", AccountsPath: filepath.Join(dir, "accounts.json")}
	module := New(context.Background(), cfg, auth.NewAccountRegistry())
	account, err := module.CreateAccount(CreateRequest{Label: "work"})
	if err != nil {
		t.Fatal(err)
	}
	if account.Authenticated || !account.ReauthenticationRequired || len(account.RequiredScopes) != 1 || account.RequiredScopes[0] != "User.Read" {
		t.Fatalf("new account state = %+v", account)
	}
}
