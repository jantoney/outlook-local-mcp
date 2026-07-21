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

// TestConcurrentCalendarAliasAddsPreserveBothMutations verifies one alias add
// cannot publish a detached pre-mutation snapshot over another completed add.
func TestConcurrentCalendarAliasAddsPreserveBothMutations(t *testing.T) {
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

	path := filepath.Join(t.TempDir(), "accounts.json")
	if err := auth.SaveAccounts(path, []auth.AccountConfig{{Label: "work"}}); err != nil {
		t.Fatalf("SaveAccounts() error = %v", err)
	}
	registry := auth.NewAccountRegistry()
	if err := registry.Add(&auth.AccountEntry{
		Label: "work", Client: client, Authenticated: true,
		TokenTenantContext: auth.TokenTenantOrganizational,
	}); err != nil {
		t.Fatalf("registry.Add() error = %v", err)
	}
	add := HandleAddCalendarAlias(registry, path, graph.RetryConfig{}, time.Second)
	type callResult struct {
		result *mcp.CallToolResult
		err    error
	}
	mountedDone := make(chan callResult, 1)
	go func() {
		result, err := add(context.Background(), calendarAliasRequest(map[string]any{
			"label": "work", "alias": "mounted", "owner": "owner@example.com",
			"kind": "mounted_calendar", "profile": "off", "mounted_calendar_id": "mounted-id",
			"confirm_mounted_selection": true,
		}))
		mountedDone <- callResult{result: result, err: err}
	}()
	<-discoveryStarted

	ownerDone := make(chan callResult, 1)
	go func() {
		result, err := add(context.Background(), calendarAliasRequest(map[string]any{
			"label": "work", "alias": "owner", "owner": "owner@example.com",
			"kind": "owner_primary_calendar", "profile": "off",
		}))
		ownerDone <- callResult{result: result, err: err}
	}()

	var ownerResult callResult
	ownerFinishedBeforeRelease := false
	select {
	case ownerResult = <-ownerDone:
		ownerFinishedBeforeRelease = true
	case <-time.After(200 * time.Millisecond):
	}
	close(releaseDiscovery)
	mountedResult := <-mountedDone
	if !ownerFinishedBeforeRelease {
		ownerResult = <-ownerDone
	}
	for name, call := range map[string]callResult{"mounted": mountedResult, "owner": ownerResult} {
		if call.err != nil || call.result == nil || call.result.IsError {
			t.Fatalf("%s add failed: result=%+v error=%v", name, call.result, call.err)
		}
	}

	entry, _ := registry.Get("work")
	if len(entry.CalendarAliases) != 2 {
		t.Fatalf("runtime aliases = %+v, want both concurrent additions", entry.CalendarAliases)
	}
	persisted, err := auth.AccountCalendarAliases(path, "work")
	if err != nil || len(persisted) != 2 {
		t.Fatalf("persisted aliases = %+v, error = %v, want both concurrent additions", persisted, err)
	}
}
