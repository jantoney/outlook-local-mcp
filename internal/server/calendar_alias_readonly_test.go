package server

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/desek/outlook-local-mcp/internal/auth"
	"github.com/desek/outlook-local-mcp/internal/graph"
	"github.com/desek/outlook-local-mcp/internal/tools"
	"github.com/mark3labs/mcp-go/mcp"
)

// TestCalendarAliasMutationReadOnlyGuard verifies read-only mode rejects alias
// configuration before local validation, persistence, or Graph discovery.
func TestCalendarAliasMutationReadOnlyGuard(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "accounts.json")
	if err := auth.SaveAccounts(path, []auth.AccountConfig{{Label: "work"}}); err != nil {
		t.Fatalf("SaveAccounts() error = %v", err)
	}
	registry := auth.NewAccountRegistry()
	if err := registry.Add(&auth.AccountEntry{
		Label: "work", TokenTenantContext: auth.TokenTenantOrganizational,
	}); err != nil {
		t.Fatalf("registry.Add() error = %v", err)
	}
	handler := ReadOnlyGuard(
		"account.add_calendar_alias", true,
		tools.HandleAddCalendarAlias(registry, path, graph.RetryConfig{}, time.Second),
	)
	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]any{
		"label": "work", "alias": "team", "owner": "owner@example.com",
		"kind": "owner_primary_calendar", "profile": "manage",
	}
	result, err := handler(context.Background(), request)
	if err != nil {
		t.Fatalf("handler error = %v", err)
	}
	if !result.IsError {
		t.Fatal("read-only mutation succeeded")
	}
	aliases, err := auth.AccountCalendarAliases(path, "work")
	if err != nil {
		t.Fatalf("AccountCalendarAliases() error = %v", err)
	}
	if len(aliases) != 0 {
		t.Fatalf("read-only mutation persisted aliases: %+v", aliases)
	}
}
