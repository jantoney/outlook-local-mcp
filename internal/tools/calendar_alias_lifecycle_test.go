package tools

import (
	"context"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/desek/outlook-local-mcp/internal/auth"
	"github.com/desek/outlook-local-mcp/internal/graph"
	"github.com/mark3labs/mcp-go/mcp"
)

// TestCalendarAliasLifecycleIsAccountScoped verifies alias reuse across
// accounts, rename identity preservation, removal, and recreate invalidation.
func TestCalendarAliasLifecycleIsAccountScoped(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "accounts.json")
	if err := auth.SaveAccounts(path, []auth.AccountConfig{
		{Label: "alpha"}, {Label: "beta"},
	}); err != nil {
		t.Fatalf("SaveAccounts() error = %v", err)
	}
	registry := auth.NewAccountRegistry()
	for _, label := range []string{"alpha", "beta"} {
		if err := registry.Add(&auth.AccountEntry{
			Label: label, TokenTenantContext: auth.TokenTenantOrganizational,
		}); err != nil {
			t.Fatalf("registry.Add(%s) error = %v", label, err)
		}
	}
	add := HandleAddCalendarAlias(registry, path, graph.RetryConfig{}, time.Second)
	for _, label := range []string{"alpha", "beta"} {
		result, err := add(context.Background(), calendarAliasRequest(map[string]any{
			"label": label, "alias": "team", "owner": label + "@example.com",
			"kind": "owner_primary_calendar", "profile": "off",
		}))
		if err != nil || result.IsError {
			t.Fatalf("add alias for %s failed: result=%v err=%v", label, result, err)
		}
	}
	alpha, _ := registry.Get("alpha")
	beta, _ := registry.Get("beta")
	if alpha.CalendarAliases[0].Owner == beta.CalendarAliases[0].Owner {
		t.Fatal("same account-scoped alias name was rebound across accounts")
	}
	originalID := alpha.CalendarAliases[0].ResourceID

	rename := HandleRenameCalendarAlias(registry, path)
	result, err := rename(context.Background(), calendarAliasRequest(map[string]any{
		"label": "alpha", "alias": "team", "new_alias": "finance",
	}))
	if err != nil || result.IsError {
		t.Fatalf("rename failed: result=%v err=%v", result, err)
	}
	alpha, _ = registry.Get("alpha")
	if alpha.CalendarAliases[0].ResourceID != originalID {
		t.Fatal("rename changed immutable resource identity")
	}

	remove := HandleRemoveCalendarAlias(registry, path)
	result, err = remove(context.Background(), calendarAliasRequest(map[string]any{
		"label": "alpha", "alias": "finance",
	}))
	if err != nil || result.IsError {
		t.Fatalf("remove failed: result=%v err=%v", result, err)
	}
	result, err = add(context.Background(), calendarAliasRequest(map[string]any{
		"label": "alpha", "alias": "finance", "owner": "alpha@example.com",
		"kind": "owner_primary_calendar", "profile": "off",
	}))
	if err != nil || result.IsError {
		t.Fatalf("recreate failed: result=%v err=%v", result, err)
	}
	alpha, _ = registry.Get("alpha")
	if alpha.CalendarAliases[0].ResourceID == originalID {
		t.Fatal("remove and recreate reused revoked resource identity")
	}
}

// TestMountedCalendarRequiresFreshExplicitSelection verifies creation filters
// discovery by owner and refuses an unconfirmed or absent mounted ID.
func TestMountedCalendarRequiresFreshExplicitSelection(t *testing.T) {
	t.Parallel()

	client, server := newTestGraphClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.EscapedPath(), "/calendars") {
			t.Errorf("request path = %q, want recipient calendar collection", r.URL.EscapedPath())
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"value":[
			{"id":"selected-id","name":"Owner calendar","canEdit":true,"owner":{"address":"owner@example.com"}},
			{"id":"other-id","name":"Other calendar","canEdit":true,"owner":{"address":"other@example.com"}}
		]}`))
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
	base := map[string]any{
		"label": "work", "alias": "mounted", "owner": "owner@example.com",
		"kind": "mounted_calendar", "profile": "off",
	}
	result, err := add(context.Background(), calendarAliasRequest(base))
	if err != nil || !result.IsError || !strings.Contains(extractText(t, result), "explicit mounted-calendar selection required") {
		t.Fatalf("unconfirmed add result=%v err=%v", result, err)
	}
	base["mounted_calendar_id"] = "other-id"
	base["confirm_mounted_selection"] = true
	result, err = add(context.Background(), calendarAliasRequest(base))
	if err != nil || !result.IsError || !strings.Contains(extractText(t, result), "not returned by fresh discovery") {
		t.Fatalf("wrong-owner selection result=%v err=%v", result, err)
	}
	base["mounted_calendar_id"] = "selected-id"
	result, err = add(context.Background(), calendarAliasRequest(base))
	if err != nil || result.IsError {
		t.Fatalf("confirmed selection result=%v err=%v", result, err)
	}
	entry, _ := registry.Get("work")
	if len(entry.CalendarAliases) != 1 || entry.CalendarAliases[0].MountedCalendarID != "selected-id" {
		t.Fatalf("persisted aliases = %+v, want selected mounted ID", entry.CalendarAliases)
	}
}

// TestCalendarAliasCompatibilityDeniedBeforeGraph verifies owner-primary manage
// and personal owner-view configuration fail without a discovery request.
func TestCalendarAliasCompatibilityDeniedBeforeGraph(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "accounts.json")
	if err := auth.SaveAccounts(path, []auth.AccountConfig{{Label: "personal"}}); err != nil {
		t.Fatalf("SaveAccounts() error = %v", err)
	}
	registry := auth.NewAccountRegistry()
	if err := registry.Add(&auth.AccountEntry{
		Label: "personal", TokenTenantContext: auth.TokenTenantPersonal,
	}); err != nil {
		t.Fatalf("registry.Add() error = %v", err)
	}
	add := HandleAddCalendarAlias(registry, path, graph.RetryConfig{}, time.Second)
	for _, profile := range []string{"read", "manage"} {
		result, err := add(context.Background(), calendarAliasRequest(map[string]any{
			"label": "personal", "alias": "owner-" + profile, "owner": "owner@example.com",
			"kind": "owner_primary_calendar", "profile": profile,
		}))
		if err != nil || !result.IsError {
			t.Fatalf("profile %s result=%v err=%v, want local denial", profile, result, err)
		}
	}
}

// calendarAliasRequest builds one aggregate-operation request for direct
// handler testing.
func calendarAliasRequest(arguments map[string]any) mcp.CallToolRequest {
	request := mcp.CallToolRequest{}
	request.Params.Arguments = arguments
	return request
}
