package server

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/desek/outlook-local-mcp/internal/auth"
	"github.com/desek/outlook-local-mcp/internal/tools"
	"github.com/mark3labs/mcp-go/mcp"
)

// TestMailAliasMutationReadOnlyGuard verifies shared-mail configuration is
// rejected before persistence when global read-only mode is active.
func TestMailAliasMutationReadOnlyGuard(t *testing.T) {
	path := filepath.Join(t.TempDir(), "accounts.json")
	if err := auth.SaveAccounts(path, []auth.AccountConfig{{Label: "work"}}); err != nil {
		t.Fatalf("SaveAccounts() error = %v", err)
	}
	registry := auth.NewAccountRegistry()
	if err := registry.Add(&auth.AccountEntry{Label: "work", TokenTenantContext: auth.TokenTenantOrganizational}); err != nil {
		t.Fatalf("registry.Add() error = %v", err)
	}
	handler := ReadOnlyGuard("account.add_mail_alias", true, tools.HandleAddMailAlias(registry, path))
	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]any{"label": "work", "alias": "finance", "owner": "finance@example.com"}
	result, err := handler(context.Background(), request)
	if err != nil || !result.IsError {
		t.Fatalf("guarded result = %+v, error = %v", result, err)
	}
	aliases, err := auth.AccountMailAliases(path, "work")
	if err != nil || len(aliases) != 0 {
		t.Fatalf("persisted aliases = %+v, error = %v", aliases, err)
	}
}
