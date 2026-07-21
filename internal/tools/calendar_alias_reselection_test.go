package tools

import (
	"context"
	"net/http"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/desek/outlook-local-mcp/internal/auth"
	"github.com/desek/outlook-local-mcp/internal/graph"
	"github.com/desek/outlook-local-mcp/internal/resource"
)

// TestReselectMountedCalendarRotatesResourceIdentity verifies a freshly
// confirmed selection preserves the workflow alias while revoking references
// minted for the prior mounted-calendar identity before any Graph item call.
func TestReselectMountedCalendarRotatesResourceIdentity(t *testing.T) {
	var discoveryCalls atomic.Int32
	client, server := newTestGraphClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		discoveryCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"value":[{"id":"mounted-2","name":"Team","canEdit":true,"owner":{"address":"owner@example.com"}}]}`))
	}))
	defer server.Close()

	path := filepath.Join(t.TempDir(), "accounts.json")
	oldID := resource.ResourceID("22222222-2222-4222-8222-222222222222")
	alias, err := resource.NewMountedCalendar(oldID, "team", "owner@example.com", "mounted-1", resource.CalendarProfileRead)
	if err != nil {
		t.Fatalf("NewMountedCalendar() error = %v", err)
	}
	aliases := []resource.CalendarAlias{alias}
	if err := auth.SaveAccounts(path, []auth.AccountConfig{{Label: "work", CalendarAliases: &aliases}}); err != nil {
		t.Fatalf("SaveAccounts() error = %v", err)
	}
	entry := &auth.AccountEntry{
		AccountID: resource.AccountID("11111111-1111-4111-8111-111111111111"),
		Label:     "work", Client: client, Authenticated: true,
		TokenTenantContext: auth.TokenTenantOrganizational, CalendarAliases: aliases,
	}
	registry := auth.NewAccountRegistry()
	if err := registry.Add(entry); err != nil {
		t.Fatalf("registry.Add() error = %v", err)
	}
	key, err := resource.LoadOrCreateSigningKey(filepath.Join(t.TempDir(), "reference.key"))
	if err != nil {
		t.Fatalf("LoadOrCreateSigningKey() error = %v", err)
	}
	codec := resource.NewReferenceCodec(key)
	oldReference, err := codec.Sign(resource.ReferenceClaims{
		AccountID: entry.AccountID, ResourceID: oldID,
		ResourceKind: resource.ResourceKindMountedCalendar, MailboxView: resource.MailboxViewRecipient,
		ItemKind:     resource.ItemKindEvent,
		GraphIDChain: []resource.GraphID{{Kind: resource.ItemKindEvent, ID: "event-1"}},
	})
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}

	reselect := HandleReselectCalendarAlias(registry, path, graph.RetryConfig{}, time.Second)
	result, err := reselect(context.Background(), calendarAliasRequest(map[string]any{
		"label": "work", "alias": "team", "mounted_calendar_id": "mounted-2",
		"confirm_mounted_selection": true,
	}))
	if err != nil || result.IsError {
		t.Fatalf("reselect failed: result=%v err=%v", result, err)
	}
	updated, _ := registry.Get("work")
	got := updated.CalendarAliases[0]
	if got.ResourceID == oldID {
		t.Fatal("reselection preserved the prior resource identity")
	}
	if got.Alias != "team" || got.Owner != alias.Owner || got.Profile != alias.Profile || got.MountedCalendarID != "mounted-2" {
		t.Fatalf("reselected alias = %+v, want stable human alias, owner, and profile with mounted-2", got)
	}
	_, resolveErr := auth.ResolveTarget(updated, auth.TargetRequest{
		Family: resource.TargetFamilyCalendar, SharedResource: "team", Capability: resource.TargetCapabilityRead,
		Reference: oldReference, RequireReference: true, ItemKind: resource.ItemKindEvent,
	}, &codec)
	if resolveErr == nil || !strings.Contains(resolveErr.Error(), "does not match the resolved target") {
		t.Fatalf("ResolveTarget(old reference) error = %v, want local target mismatch", resolveErr)
	}
	if discoveryCalls.Load() != 1 {
		t.Fatalf("Graph calls = %d, want only fresh calendar discovery", discoveryCalls.Load())
	}
}
