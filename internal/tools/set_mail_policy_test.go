package tools

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/desek/outlook-local-mcp/internal/auth"
	"github.com/mark3labs/mcp-go/mcp"
	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"
)

// TestSetMailPolicyAppliesSameScopeChangeWithoutDisconnect verifies an exact
// policy change is persisted and visible immediately when its OAuth scope set
// is unchanged.
func TestSetMailPolicyAppliesSameScopeChangeWithoutDisconnect(t *testing.T) {
	dir := t.TempDir()
	accountsPath := filepath.Join(dir, "accounts.json")
	oldPolicy := auth.MailActionPolicy{Draft: true}
	if err := auth.SaveAccounts(accountsPath, []auth.AccountConfig{{
		Label: "work", ClientID: "client", TenantID: "tenant", AuthMethod: "browser", MailPolicy: &oldPolicy,
	}}); err != nil {
		t.Fatal(err)
	}
	registry := auth.NewAccountRegistry()
	client := &msgraphsdk.GraphServiceClient{}
	if err := registry.Add(&auth.AccountEntry{
		Label: "work", ClientID: "client", TenantID: "tenant", AuthMethod: "browser",
		Authenticated: true, Client: client, MailPolicy: oldPolicy,
		Scopes: auth.ScopesForMailPolicy(oldPolicy),
	}); err != nil {
		t.Fatal(err)
	}
	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]any{
		"label": "work", "draft": false, "archive": true,
	}
	result, err := HandleSetMailPolicy(registry, accountsPath)(context.Background(), request)
	if err != nil || result.IsError {
		t.Fatalf("HandleSetMailPolicy() = (%v, %v)", result, err)
	}
	entry, _ := registry.Get("work")
	want := auth.MailActionPolicy{Archive: true}
	if entry.MailPolicy != want || !entry.Authenticated || entry.Client != client {
		t.Fatalf("runtime entry = %+v, want connected policy %+v", entry, want)
	}
	accounts, err := auth.LoadAccounts(accountsPath)
	if err != nil || accounts[0].MailPolicy == nil || *accounts[0].MailPolicy != want {
		t.Fatalf("persisted accounts = %+v, err = %v", accounts, err)
	}
}
