package tools

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/desek/outlook-local-mcp/internal/auth"
	"github.com/desek/outlook-local-mcp/internal/resource"
	"github.com/mark3labs/mcp-go/mcp"
)

// TestMailAliasLifecycle verifies default-off creation, independent policy,
// rename identity preservation, removal, and recreate identity invalidation.
func TestMailAliasLifecycle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "accounts.json")
	if err := auth.SaveAccounts(path, []auth.AccountConfig{{Label: "work"}}); err != nil {
		t.Fatalf("SaveAccounts() error = %v", err)
	}
	registry := auth.NewAccountRegistry()
	if err := registry.Add(&auth.AccountEntry{Label: "work", TokenTenantContext: auth.TokenTenantOrganizational}); err != nil {
		t.Fatalf("registry.Add() error = %v", err)
	}
	call := func(handler func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error), args map[string]any) *mcp.CallToolResult {
		request := mcp.CallToolRequest{}
		request.Params.Arguments = args
		result, err := handler(context.Background(), request)
		if err != nil {
			t.Fatalf("handler error = %v", err)
		}
		return result
	}
	added := call(HandleAddMailAlias(registry, path), map[string]any{"label": "work", "alias": "finance", "owner": "finance@example.com"})
	if added.IsError {
		t.Fatalf("add result = %s", extractText(t, added))
	}
	entry, _ := registry.Get("work")
	if len(entry.MailAliases) != 1 || entry.MailAliases[0].Policy != (auth.MailActionPolicy{}) {
		t.Fatalf("new aliases = %+v, want one default-off alias", entry.MailAliases)
	}
	oldID := entry.MailAliases[0].ResourceID
	updated := call(HandleSetMailAliasPolicy(registry, path), map[string]any{"label": "work", "alias": "finance", "archive": true})
	if updated.IsError {
		t.Fatalf("policy result = %s", extractText(t, updated))
	}
	entry, _ = registry.Get("work")
	if !entry.MailAliases[0].Policy.Archive || entry.MailPolicy.Archive {
		t.Fatalf("shared policy = %+v own policy = %+v", entry.MailAliases[0].Policy, entry.MailPolicy)
	}
	renamed := call(HandleRenameMailAlias(registry, path), map[string]any{"label": "work", "alias": "finance", "new_alias": "finance-team"})
	if renamed.IsError {
		t.Fatalf("rename result = %s", extractText(t, renamed))
	}
	entry, _ = registry.Get("work")
	if entry.MailAliases[0].ResourceID != oldID {
		t.Fatalf("rename resource ID = %s, want %s", entry.MailAliases[0].ResourceID, oldID)
	}
	_ = call(HandleRemoveMailAlias(registry, path), map[string]any{"label": "work", "alias": "finance-team"})
	_ = call(HandleAddMailAlias(registry, path), map[string]any{"label": "work", "alias": "finance-team", "owner": "finance@example.com"})
	entry, _ = registry.Get("work")
	if len(entry.MailAliases) != 1 || entry.MailAliases[0].ResourceID == oldID {
		t.Fatalf("recreated alias = %+v, want new identity", entry.MailAliases)
	}
}

// TestMailAliasConfigurationRejectsIncompatibleContexts verifies personal and
// unknown contexts fail locally before any shared-mail configuration persists.
func TestMailAliasConfigurationRejectsIncompatibleContexts(t *testing.T) {
	for _, tenantContext := range []auth.TokenTenantContext{auth.TokenTenantPersonal, auth.TokenTenantUnknown} {
		t.Run(string(tenantContext), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "accounts.json")
			if err := auth.SaveAccounts(path, []auth.AccountConfig{{Label: "target"}}); err != nil {
				t.Fatalf("SaveAccounts() error = %v", err)
			}
			registry := auth.NewAccountRegistry()
			if err := registry.Add(&auth.AccountEntry{Label: "target", TokenTenantContext: tenantContext}); err != nil {
				t.Fatalf("registry.Add() error = %v", err)
			}
			request := mcp.CallToolRequest{}
			request.Params.Arguments = map[string]any{"label": "target", "alias": "shared", "owner": "shared@example.com"}
			result, err := HandleAddMailAlias(registry, path)(context.Background(), request)
			if err != nil || !result.IsError || !strings.Contains(extractText(t, result), "organizational") {
				t.Fatalf("incompatible result = %+v, error = %v", result, err)
			}
			aliases, err := auth.AccountMailAliases(path, "target")
			if err != nil || len(aliases) != 0 {
				t.Fatalf("persisted aliases = %+v, error = %v", aliases, err)
			}
		})
	}
}

// TestMailAliasAndCalendarAliasNamesAreIndependent verifies identical human
// selectors may exist in the two deliberately separate resource families.
func TestMailAliasAndCalendarAliasNamesAreIndependent(t *testing.T) {
	id, err := resource.NewResourceID()
	if err != nil {
		t.Fatalf("NewResourceID() error = %v", err)
	}
	calendar, err := resource.NewOwnerPrimaryCalendar(id, "finance", "finance@example.com", resource.CalendarProfileOff)
	if err != nil {
		t.Fatalf("NewOwnerPrimaryCalendar() error = %v", err)
	}
	path := filepath.Join(t.TempDir(), "accounts.json")
	calendarAliases := []resource.CalendarAlias{calendar}
	if err := auth.SaveAccounts(path, []auth.AccountConfig{{Label: "work", CalendarAliases: &calendarAliases}}); err != nil {
		t.Fatalf("SaveAccounts() error = %v", err)
	}
	registry := auth.NewAccountRegistry()
	if err := registry.Add(&auth.AccountEntry{Label: "work", TokenTenantContext: auth.TokenTenantOrganizational, CalendarAliases: calendarAliases}); err != nil {
		t.Fatalf("registry.Add() error = %v", err)
	}
	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]any{"label": "work", "alias": "finance", "owner": "finance@example.com"}
	result, err := HandleAddMailAlias(registry, path)(context.Background(), request)
	if err != nil || result.IsError {
		t.Fatalf("same-name mail add result = %+v, error = %v", result, err)
	}
}
