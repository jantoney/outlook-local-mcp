package tools

import (
	"context"
	"net/http"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/desek/outlook-local-mcp/internal/auth"
	"github.com/desek/outlook-local-mcp/internal/graph"
	"github.com/mark3labs/mcp-go/mcp"
)

// TestRemoveAccountWaitsForAliasMutationTransaction verifies account removal
// cannot reuse a label while an older account's alias transaction is between
// its read, persistence, and runtime-publication stages.
func TestRemoveAccountWaitsForAliasMutationTransaction(t *testing.T) {
	discoveryStarted := make(chan struct{})
	releaseDiscovery := make(chan struct{})
	var signalOnce sync.Once
	client, server := newTestGraphClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		signalOnce.Do(func() { close(discoveryStarted) })
		<-releaseDiscovery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"value":[{"id":"mounted-id","name":"Team","canEdit":true,"owner":{"address":"owner@example.com"}}]}`))
	}))
	defer server.Close()

	oldID := auth.AccountID("11111111-1111-4111-8111-111111111111")
	path := filepath.Join(t.TempDir(), "accounts.json")
	if err := auth.SaveAccounts(path, []auth.AccountConfig{{AccountID: oldID, Label: "work"}}); err != nil {
		t.Fatalf("SaveAccounts() error = %v", err)
	}
	registry := auth.NewAccountRegistry()
	if err := registry.Add(&auth.AccountEntry{
		AccountID: oldID, Label: "work", Client: client, Authenticated: true,
		TokenTenantContext: auth.TokenTenantOrganizational,
	}); err != nil {
		t.Fatalf("registry.Add() error = %v", err)
	}

	addAlias := HandleAddCalendarAlias(registry, path, graph.RetryConfig{}, time.Second)
	removeAccount := HandleRemoveAccount(registry, path)
	type callResult struct {
		result *mcp.CallToolResult
		err    error
	}
	aliasDone := make(chan callResult, 1)
	go func() {
		result, err := addAlias(context.Background(), calendarAliasRequest(map[string]any{
			"label": "work", "alias": "mounted", "owner": "owner@example.com",
			"kind": "mounted_calendar", "profile": "off", "mounted_calendar_id": "mounted-id",
			"confirm_mounted_selection": true,
		}))
		aliasDone <- callResult{result: result, err: err}
	}()
	<-discoveryStarted

	removeDone := make(chan callResult, 1)
	go func() {
		result, err := removeAccount(context.Background(), mcp.CallToolRequest{Params: mcp.CallToolParams{
			Arguments: map[string]any{"label": "work"},
		}})
		removeDone <- callResult{result: result, err: err}
	}()
	select {
	case result := <-removeDone:
		t.Fatalf("account removal escaped the alias transaction lock: result=%+v error=%v", result.result, result.err)
	case <-time.After(100 * time.Millisecond):
	}

	close(releaseDiscovery)
	for name, call := range map[string]callResult{"alias": <-aliasDone, "remove": <-removeDone} {
		if call.err != nil || call.result == nil || call.result.IsError {
			t.Fatalf("%s operation failed: result=%+v error=%v", name, call.result, call.err)
		}
	}
	if _, exists := registry.Get("work"); exists {
		t.Fatal("removed account remains in runtime registry")
	}
	accounts, err := auth.LoadAccounts(path)
	if err != nil || len(accounts) != 0 {
		t.Fatalf("persisted accounts = %+v, error = %v, want empty account set", accounts, err)
	}
}
