package server

import (
	"context"
	"net/http"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/desek/outlook-local-mcp/internal/auth"
	"github.com/desek/outlook-local-mcp/internal/resource"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"
)

// TestSharedMailReadGuardFailsBeforeHandlerAndTransport verifies incompatible,
// disabled, removed, wrong-kind, and cross-target reads fail locally.
func TestSharedMailReadGuardFailsBeforeHandlerAndTransport(t *testing.T) {
	var handlerCalls atomic.Int32
	var transportCalls atomic.Int32
	client, server := newServerTestGraphClient(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { transportCalls.Add(1) }))
	defer server.Close()
	inner := mcpserver.ToolHandlerFunc(func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		handlerCalls.Add(1)
		return mcp.NewToolResultText("unexpected"), nil
	})
	tests := []struct {
		name   string
		tenant auth.TokenTenantContext
		setup  func(*auth.AccountEntry)
	}{
		{"personal", auth.TokenTenantPersonal, enableSharedMailRead},
		{"unknown", auth.TokenTenantUnknown, enableSharedMailRead},
		{"disabled", auth.TokenTenantOrganizational, func(*auth.AccountEntry) {}},
		{"removed", auth.TokenTenantOrganizational, func(entry *auth.AccountEntry) { entry.MailAliases = nil }},
		{"wrong kind", auth.TokenTenantOrganizational, func(entry *auth.AccountEntry) {
			entry.MailAliases = nil
			alias, err := resource.NewOwnerPrimaryCalendar(resource.ResourceID("33333333-3333-4333-8333-333333333333"), "finance-mail", "shared@example.com", resource.CalendarProfileRead)
			if err != nil {
				t.Fatalf("NewOwnerPrimaryCalendar() error = %v", err)
			}
			entry.CalendarAliases = []resource.CalendarAlias{alias}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			registry, entry, codec := sharedMailGuardFixture(t, client, test.tenant)
			test.setup(entry)
			publishSharedMailGuardEntry(t, registry, entry)
			request := requestWithArguments(map[string]any{"shared_resource": "finance-mail"})
			result, err := ResolvedTargetGuard(registry, mailSharedReadGuard(), &codec, inner)(ownerGuardAccountContext(entry), request)
			if err != nil || !result.IsError {
				t.Fatalf("guard result = %+v, error = %v", result, err)
			}
		})
	}

	t.Run("cross target reference", func(t *testing.T) {
		registry, entry, codec := sharedMailGuardFixture(t, client, auth.TokenTenantOrganizational)
		enableSharedMailRead(entry)
		other, err := resource.NewMailAlias(resource.ResourceID("44444444-4444-4444-8444-444444444444"), "other-mail", "other@example.com")
		if err != nil {
			t.Fatalf("NewMailAlias() error = %v", err)
		}
		other.Policy.Read = true
		entry.MailAliases = append(entry.MailAliases, other)
		publishSharedMailGuardEntry(t, registry, entry)
		otherTarget, err := resource.TargetFromMailAlias(entry.AccountID, other)
		if err != nil {
			t.Fatalf("TargetFromMailAlias() error = %v", err)
		}
		reference, err := codec.Sign(resource.ReferenceClaims{
			AccountID: otherTarget.AccountID, ResourceID: otherTarget.ResourceID,
			ResourceKind: otherTarget.Kind, MailboxView: otherTarget.View, ItemKind: resource.ItemKindMessage,
			GraphIDChain: []resource.GraphID{{Kind: resource.ItemKindMessage, ID: "message-1"}},
		})
		if err != nil {
			t.Fatalf("Sign() error = %v", err)
		}
		request := requestWithArguments(map[string]any{"shared_resource": "finance-mail", "resource_ref": reference})
		result, err := ResolvedTargetGuard(registry, mailSharedMessageReadGuard(), &codec, inner)(ownerGuardAccountContext(entry), request)
		if err != nil || !result.IsError {
			t.Fatalf("cross-target result = %+v, error = %v", result, err)
		}
	})
	if handlerCalls.Load() != 0 || transportCalls.Load() != 0 {
		t.Fatalf("handler calls = %d, transport calls = %d; want zero", handlerCalls.Load(), transportCalls.Load())
	}
}

// TestSharedMailReadGuardRecordsOwnerAuditTarget verifies successful routing
// publishes the immutable mailbox identity and exact local policy for audit.
func TestSharedMailReadGuardRecordsOwnerAuditTarget(t *testing.T) {
	client, server := newServerTestGraphClient(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer server.Close()
	registry, entry, codec := sharedMailGuardFixture(t, client, auth.TokenTenantOrganizational)
	enableSharedMailRead(entry)
	publishSharedMailGuardEntry(t, registry, entry)
	inner := mcpserver.ToolHandlerFunc(func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		target, ok := auth.AuditTargetFromContext(ctx)
		if !ok || target.Alias != "finance-mail" || target.ResourceKind != resource.ResourceKindMailbox ||
			target.MailboxView != resource.MailboxViewOwner || !target.MailPolicy.Read || target.Compatibility != "compatible" {
			t.Fatalf("audit target = %+v", target)
		}
		return mcp.NewToolResultText("ok"), nil
	})
	request := requestWithArguments(map[string]any{"shared_resource": "finance-mail"})
	ctx := auth.WithAuditTargetRecorder(ownerGuardAccountContext(entry))
	result, err := ResolvedTargetGuard(registry, mailSharedReadGuard(), &codec, inner)(ctx, request)
	if err != nil || result.IsError {
		t.Fatalf("guard result = %+v, error = %v", result, err)
	}
}

// TestSharedDraftGuardsFailBeforeHandler verifies exact draft capability and
// target-bound message or draft provenance are required before any Graph work.
func TestSharedDraftGuardsFailBeforeHandler(t *testing.T) {
	var handlerCalls atomic.Int32
	client, server := newServerTestGraphClient(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer server.Close()
	inner := mcpserver.ToolHandlerFunc(func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		handlerCalls.Add(1)
		return mcp.NewToolResultText("unexpected"), nil
	})
	registry, entry, codec := sharedMailGuardFixture(t, client, auth.TokenTenantOrganizational)

	tests := []struct {
		name  string
		guard TargetGuardConfig
		args  map[string]any
	}{
		{"draft disabled", mailSharedDraftCreateGuard(), map[string]any{"shared_resource": "finance-mail"}},
		{"reply raw id", mailSharedDraftSourceGuard(), map[string]any{"shared_resource": "finance-mail", "message_id": "message-1"}},
		{"update raw id", mailSharedDraftItemGuard(), map[string]any{"shared_resource": "finance-mail", "message_id": "draft-1"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := ResolvedTargetGuard(registry, test.guard, &codec, inner)(ownerGuardAccountContext(entry), requestWithArguments(test.args))
			if err != nil || !result.IsError {
				t.Fatalf("result = %+v, error = %v", result, err)
			}
		})
	}

	entry.MailAliases[0].Policy.Draft = true
	other, err := resource.NewMailAlias(resource.ResourceID("44444444-4444-4444-8444-444444444444"), "other-mail", "other@example.com")
	if err != nil {
		t.Fatalf("NewMailAlias() error = %v", err)
	}
	other.Policy.Draft = true
	entry.MailAliases = append(entry.MailAliases, other)
	publishSharedMailGuardEntry(t, registry, entry)
	otherTarget, err := resource.TargetFromMailAlias(entry.AccountID, other)
	if err != nil {
		t.Fatalf("TargetFromMailAlias() error = %v", err)
	}
	reference, err := codec.Sign(resource.ReferenceClaims{
		AccountID: otherTarget.AccountID, ResourceID: otherTarget.ResourceID, ResourceKind: otherTarget.Kind,
		MailboxView: otherTarget.View, ItemKind: resource.ItemKindDraft,
		GraphIDChain: []resource.GraphID{{Kind: resource.ItemKindDraft, ID: "draft-1"}},
	})
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}
	result, err := ResolvedTargetGuard(registry, mailSharedDraftItemGuard(), &codec, inner)(ownerGuardAccountContext(entry), requestWithArguments(map[string]any{
		"shared_resource": "finance-mail", "draft_ref": reference,
	}))
	if err != nil || !result.IsError {
		t.Fatalf("cross-target result = %+v, error = %v", result, err)
	}
	if handlerCalls.Load() != 0 {
		t.Fatalf("handler calls = %d, want zero", handlerCalls.Load())
	}
}

// TestMoveMessageGuardRequiresEnabledTargetBoundSource verifies move requests
// cannot reach a handler with a disabled policy, a raw ID, or cross-target
// source provenance.
func TestMoveMessageGuardRequiresEnabledTargetBoundSource(t *testing.T) {
	var handlerCalls atomic.Int32
	client, server := newServerTestGraphClient(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer server.Close()
	inner := mcpserver.ToolHandlerFunc(func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		handlerCalls.Add(1)
		return mcp.NewToolResultText("unexpected"), nil
	})
	registry, entry, codec := sharedMailGuardFixture(t, client, auth.TokenTenantOrganizational)

	for _, args := range []map[string]any{
		{"shared_resource": "finance-mail", "message_id": "message-1"},
		{"shared_resource": "finance-mail"},
	} {
		result, err := ResolvedTargetGuard(registry, mailMoveMessageGuard(), &codec, inner)(ownerGuardAccountContext(entry), requestWithArguments(args))
		if err != nil || !result.IsError {
			t.Fatalf("disabled/raw-ID result = %+v, error = %v", result, err)
		}
	}

	entry.MailAliases[0].Policy.Move = true
	other, err := resource.NewMailAlias(resource.ResourceID("44444444-4444-4444-8444-444444444444"), "other-mail", "other@example.com")
	if err != nil {
		t.Fatal(err)
	}
	other.Policy.Move = true
	entry.MailAliases = append(entry.MailAliases, other)
	publishSharedMailGuardEntry(t, registry, entry)
	otherTarget, err := resource.TargetFromMailAlias(entry.AccountID, other)
	if err != nil {
		t.Fatal(err)
	}
	reference, err := codec.Sign(resource.ReferenceClaims{
		AccountID: otherTarget.AccountID, ResourceID: otherTarget.ResourceID, ResourceKind: otherTarget.Kind,
		MailboxView: otherTarget.View, ItemKind: resource.ItemKindMessage,
		GraphIDChain: []resource.GraphID{{Kind: resource.ItemKindMessage, ID: "message-1"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := ResolvedTargetGuard(registry, mailMoveMessageGuard(), &codec, inner)(ownerGuardAccountContext(entry), requestWithArguments(map[string]any{
		"shared_resource": "finance-mail", "message_ref": reference,
	}))
	if err != nil || !result.IsError {
		t.Fatalf("cross-target result = %+v, error = %v", result, err)
	}
	if handlerCalls.Load() != 0 {
		t.Fatalf("handler calls = %d, want zero", handlerCalls.Load())
	}
}

// TestSharedSendGuardRequiresSendAndDraftReference verifies shared send cannot
// use draft capability, a raw ID, or an unrelated target reference.
func TestSharedSendGuardRequiresSendAndDraftReference(t *testing.T) {
	var handlerCalls atomic.Int32
	client, server := newServerTestGraphClient(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer server.Close()
	inner := mcpserver.ToolHandlerFunc(func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		handlerCalls.Add(1)
		return mcp.NewToolResultText("ok"), nil
	})
	registry, entry, codec := sharedMailGuardFixture(t, client, auth.TokenTenantOrganizational)
	entry.MailAliases[0].Policy.Draft = true
	publishSharedMailGuardEntry(t, registry, entry)
	result, err := ResolvedTargetGuard(registry, mailSendDraftGuard(), &codec, inner)(ownerGuardAccountContext(entry), requestWithArguments(map[string]any{
		"shared_resource": "finance-mail", "message_id": "draft-1",
	}))
	if err != nil || !result.IsError || handlerCalls.Load() != 0 {
		t.Fatalf("draft-only/raw result = %+v, calls = %d, error = %v", result, handlerCalls.Load(), err)
	}
	entry.MailAliases[0].Policy.Send = true
	publishSharedMailGuardEntry(t, registry, entry)
	target, err := resource.TargetFromMailAlias(entry.AccountID, entry.MailAliases[0])
	if err != nil {
		t.Fatal(err)
	}
	reference, err := codec.Sign(resource.ReferenceClaims{
		AccountID: target.AccountID, ResourceID: target.ResourceID, ResourceKind: target.Kind,
		MailboxView: target.View, ItemKind: resource.ItemKindDraft,
		GraphIDChain: []resource.GraphID{{Kind: resource.ItemKindDraft, ID: "draft-1"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err = ResolvedTargetGuard(registry, mailSendDraftGuard(), &codec, inner)(ownerGuardAccountContext(entry), requestWithArguments(map[string]any{
		"shared_resource": "finance-mail", "draft_ref": reference,
	}))
	if err != nil || result.IsError || handlerCalls.Load() != 1 {
		t.Fatalf("enabled result = %+v, calls = %d, error = %v", result, handlerCalls.Load(), err)
	}
}

// sharedMailGuardFixture creates one organizational mailbox alias and signer.
func sharedMailGuardFixture(t *testing.T, client *msgraphsdk.GraphServiceClient, tenant auth.TokenTenantContext) (*auth.AccountRegistry, *auth.AccountEntry, resource.ReferenceCodec) {
	t.Helper()
	key, err := resource.LoadOrCreateSigningKey(filepath.Join(t.TempDir(), "reference.key"))
	if err != nil {
		t.Fatalf("LoadOrCreateSigningKey() error = %v", err)
	}
	alias, err := resource.NewMailAlias(resource.ResourceID("22222222-2222-4222-8222-222222222222"), "finance-mail", "shared@example.com")
	if err != nil {
		t.Fatalf("NewMailAlias() error = %v", err)
	}
	alias.Validation = resource.Validation{Status: resource.ValidationAvailable}
	entry := &auth.AccountEntry{
		AccountID: auth.AccountID("11111111-1111-4111-8111-111111111111"), Label: "work",
		Client: client, Authenticated: true, TokenTenantContext: tenant, MailAliases: []resource.MailAlias{alias},
	}
	registry := auth.NewAccountRegistry()
	if err := registry.Add(entry); err != nil {
		t.Fatalf("registry.Add() error = %v", err)
	}
	return registry, entry, resource.NewReferenceCodec(key)
}

// enableSharedMailRead enables only the alias-local read action.
func enableSharedMailRead(entry *auth.AccountEntry) { entry.MailAliases[0].Policy.Read = true }

// publishSharedMailGuardEntry applies fixture policy changes through the same
// locked registry mutation boundary used by production policy updates.
func publishSharedMailGuardEntry(t *testing.T, registry *auth.AccountRegistry, entry *auth.AccountEntry) {
	t.Helper()
	if err := registry.Update(entry.Label, func(current *auth.AccountEntry) {
		current.TokenTenantContext = entry.TokenTenantContext
		current.CalendarAliases = append([]resource.CalendarAlias(nil), entry.CalendarAliases...)
		current.MailAliases = append([]resource.MailAlias(nil), entry.MailAliases...)
	}); err != nil {
		t.Fatalf("registry.Update() error = %v", err)
	}
}
