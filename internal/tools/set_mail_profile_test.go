package tools

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/desek/outlook-local-mcp/internal/auth"
	"github.com/mark3labs/mcp-go/mcp"
)

// TestSetMailProfileRequiresReauth verifies that changing a profile persists
// the selection, clears the auth record, and disconnects the runtime account.
func TestSetMailProfileRequiresReauth(t *testing.T) {
	dir := t.TempDir()
	accountsPath := filepath.Join(dir, "accounts.json")
	authRecordPath := filepath.Join(dir, "work-auth.json")
	if err := os.WriteFile(authRecordPath, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := auth.SaveAccounts(accountsPath, []auth.AccountConfig{{
		Label: "work", ClientID: "client", TenantID: "tenant", AuthMethod: "browser", MailProfile: "mail_manage",
	}}); err != nil {
		t.Fatal(err)
	}
	registry := auth.NewAccountRegistry()
	if err := registry.Add(&auth.AccountEntry{
		Label: "work", ClientID: "client", TenantID: "tenant", AuthMethod: "browser",
		Authenticated: true, AuthRecordPath: authRecordPath, MailProfile: auth.MailProfileManage,
		MailPolicy: auth.MailPolicyFromProfile(auth.MailProfileManage),
	}); err != nil {
		t.Fatal(err)
	}
	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]any{"label": "work", "mail_profile": "mail_read"}
	result, err := HandleSetMailProfile(registry, accountsPath)(context.Background(), request)
	if err != nil || result.IsError {
		t.Fatalf("HandleSetMailProfile() = (%v, %v)", result, err)
	}
	entry, _ := registry.Get("work")
	if entry.Authenticated || entry.MailPolicy != (auth.MailActionPolicy{Read: true}) || entry.Client != nil {
		t.Fatalf("entry not safely disconnected: %+v", entry)
	}
	if _, err := os.Stat(authRecordPath); !os.IsNotExist(err) {
		t.Fatalf("auth record still exists: %v", err)
	}
	accounts, err := auth.LoadAccounts(accountsPath)
	if err != nil || len(accounts) != 1 || accounts[0].MailPolicy == nil || *accounts[0].MailPolicy != (auth.MailActionPolicy{Read: true}) || accounts[0].MailProfile != "" {
		t.Fatalf("persisted accounts = %+v, err = %v", accounts, err)
	}
}
