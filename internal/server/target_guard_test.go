package server

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/desek/outlook-local-mcp/internal/audit"
	"github.com/desek/outlook-local-mcp/internal/auth"
	"github.com/desek/outlook-local-mcp/internal/graph"
	"github.com/desek/outlook-local-mcp/internal/resource"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"
)

// TestResolvedTargetGuardPublishesContextAfterSuccess verifies the handler sees
// one authorized typed route only after every local check succeeds.
func TestResolvedTargetGuardPublishesContextAfterSuccess(t *testing.T) {
	var handlerCalls atomic.Int32
	var transportCalls atomic.Int32
	client, server := newServerTestGraphClient(t, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		transportCalls.Add(1)
		if request.URL.EscapedPath() != "/v1.0/users/finance%2Bops%40example.com/messages/message%2B1" {
			t.Errorf("escaped path = %q", request.URL.EscapedPath())
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"message+1"}`))
	}))
	defer server.Close()
	registry, entry, codec := targetGuardFixture(t, client)
	handler := func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		handlerCalls.Add(1)
		routed, ok := graph.RoutedTargetFromContext(ctx)
		if !ok || routed.Target.Alias != "finance" || routed.Claims == nil {
			t.Fatalf("routed target = %+v, present = %t", routed, ok)
		}
		if _, err := routed.Root.Messages().ByMessageId(routed.Claims.GraphIDChain[0].ID).Get(ctx, nil); err != nil {
			t.Fatalf("message Get() error = %v", err)
		}
		return mcp.NewToolResultText("ok"), nil
	}
	wrapped := ResolvedTargetGuard(registry, mailReadReferenceGuard(), &codec, mcpserver.ToolHandlerFunc(handler))
	request := targetGuardRequest("finance", signedTargetMessage(t, codec, entry, "message+1"))
	result, err := wrapped(targetGuardAccountContext(entry), request)
	if err != nil || result.IsError {
		t.Fatalf("guard result = %+v, error = %v", result, err)
	}
	if handlerCalls.Load() != 1 || transportCalls.Load() != 1 {
		t.Fatalf("handler calls = %d, transport calls = %d", handlerCalls.Load(), transportCalls.Load())
	}
}

// mailReadReferenceGuard returns the exact contract for reading one referenced
// shared or own message.
func mailReadReferenceGuard() TargetGuardConfig {
	return TargetGuardConfig{
		Family: resource.TargetFamilyMail, Capability: resource.TargetCapabilityRead,
		ReferenceArgument: "resource_ref", RequireReference: true, ItemKind: resource.ItemKindMessage,
	}
}

// targetGuardRequest builds one explicit shared-resource request.
func targetGuardRequest(alias, reference string) mcp.CallToolRequest {
	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]any{"shared_resource": alias, "resource_ref": reference}
	return request
}

// targetGuardAccountContext provides the account resolver output expected by
// target middleware.
func targetGuardAccountContext(entry *auth.AccountEntry) context.Context {
	return auth.WithAccountInfo(context.Background(), auth.AccountInfo{
		AccountID: entry.AccountID, Label: entry.Label,
		MailPolicy: entry.MailPolicy, TokenTenantContext: entry.TokenTenantContext,
	})
}

// targetGuardFixture constructs one organizational account, shared mailbox,
// and signing authority without making Graph traffic.
func targetGuardFixture(t *testing.T, client *msgraphsdk.GraphServiceClient) (*auth.AccountRegistry, *auth.AccountEntry, resource.ReferenceCodec) {
	t.Helper()
	key, err := resource.LoadOrCreateSigningKey(filepath.Join(t.TempDir(), "reference.key"))
	if err != nil {
		t.Fatalf("LoadOrCreateSigningKey() error = %v", err)
	}
	registry := auth.NewAccountRegistry()
	entry := &auth.AccountEntry{
		AccountID: auth.AccountID("11111111-1111-4111-8111-111111111111"),
		Label:     "work", Client: client, Authenticated: true,
		TokenTenantContext: auth.TokenTenantOrganizational,
	}
	if err := registry.Add(entry); err != nil {
		t.Fatalf("registry.Add() error = %v", err)
	}
	setFixturePolicy(t, registry, resource.MailActionPolicy{Read: true}, auth.TokenTenantOrganizational, client)
	return registry, entry, resource.NewReferenceCodec(key)
}

// setFixturePolicy replaces the exact alias policy and account compatibility
// through the registry's synchronized update seam.
func setFixturePolicy(t *testing.T, registry *auth.AccountRegistry, policy resource.MailActionPolicy, tenantContext auth.TokenTenantContext, client *msgraphsdk.GraphServiceClient) {
	t.Helper()
	id := resource.ResourceID("22222222-2222-4222-8222-222222222222")
	alias, err := resource.NewMailAlias(id, "finance", "finance+ops@example.com")
	if err != nil {
		t.Fatalf("NewMailAlias() error = %v", err)
	}
	alias.Policy = policy
	alias.Validation = resource.Validation{Status: resource.ValidationAvailable}
	setFixtureAliases(t, registry, []resource.MailAlias{alias}, tenantContext, client)
}

// setFixtureAliases replaces fixture alias, tenant, and client state.
func setFixtureAliases(t *testing.T, registry *auth.AccountRegistry, aliases []resource.MailAlias, tenantContext auth.TokenTenantContext, client *msgraphsdk.GraphServiceClient) {
	t.Helper()
	if err := registry.Update("work", func(entry *auth.AccountEntry) {
		entry.MailAliases = append([]resource.MailAlias(nil), aliases...)
		entry.TokenTenantContext = tenantContext
		entry.Client = client
	}); err != nil {
		t.Fatalf("registry.Update() error = %v", err)
	}
}

// signedTargetMessage creates a valid message reference bound to the fixture's
// immutable account and shared-mail resource identities.
func signedTargetMessage(t *testing.T, codec resource.ReferenceCodec, entry *auth.AccountEntry, messageID string) string {
	t.Helper()
	reference, err := codec.Sign(resource.ReferenceClaims{
		AccountID:    resource.AccountID(entry.AccountID),
		ResourceID:   resource.ResourceID("22222222-2222-4222-8222-222222222222"),
		ResourceKind: resource.ResourceKindMailbox, MailboxView: resource.MailboxViewOwner,
		ItemKind:     resource.ItemKindMessage,
		GraphIDChain: []resource.GraphID{{Kind: resource.ItemKindMessage, ID: messageID}},
	})
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}
	return reference
}

// TestResolvedTargetGuardChainFailsClosed verifies read-only, unknown, removed,
// disabled, incompatible, invalid-reference, and nil-client requests never
// reach a handler or transport while AuditWrap records exactly one attempt.
func TestResolvedTargetGuardChainFailsClosed(t *testing.T) {
	var handlerCalls atomic.Int32
	var transportCalls atomic.Int32
	client, server := newServerTestGraphClient(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		transportCalls.Add(1)
	}))
	defer server.Close()
	registry, entry, codec := targetGuardFixture(t, client)
	validReference := signedTargetMessage(t, codec, entry, "message+1")
	handler := mcpserver.ToolHandlerFunc(func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		handlerCalls.Add(1)
		return mcp.NewToolResultText("unexpected"), nil
	})

	tests := []struct {
		name     string
		prepare  func()
		request  mcp.CallToolRequest
		readOnly bool
	}{
		{name: "unknown", request: targetGuardRequest("missing", validReference)},
		{name: "removed", prepare: func() { setFixtureAliases(t, registry, nil, auth.TokenTenantOrganizational, client) }, request: targetGuardRequest("finance", validReference)},
		{name: "disabled", prepare: func() {
			setFixturePolicy(t, registry, resource.MailActionPolicy{}, auth.TokenTenantOrganizational, client)
		}, request: targetGuardRequest("finance", validReference)},
		{name: "incompatible", prepare: func() {
			setFixturePolicy(t, registry, resource.MailActionPolicy{Read: true}, auth.TokenTenantPersonal, client)
		}, request: targetGuardRequest("finance", validReference)},
		{name: "read only", prepare: func() {
			setFixturePolicy(t, registry, resource.MailActionPolicy{Read: true}, auth.TokenTenantOrganizational, client)
		}, request: targetGuardRequest("finance", validReference), readOnly: true},
		{name: "invalid reference", request: targetGuardRequest("finance", validReference+"tampered")},
		{name: "nil client", prepare: func() {
			setFixturePolicy(t, registry, resource.MailActionPolicy{Read: true}, auth.TokenTenantOrganizational, nil)
		}, request: targetGuardRequest("finance", validReference)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			setFixturePolicy(t, registry, resource.MailActionPolicy{Read: true}, auth.TokenTenantOrganizational, client)
			if test.prepare != nil {
				test.prepare()
			}
			auditPath := filepath.Join(t.TempDir(), "audit.jsonl")
			audit.InitAuditLog(true, auditPath)
			guarded := ResolvedTargetGuard(registry, mailReadReferenceGuard(), &codec, handler)
			var chain mcpserver.ToolHandlerFunc = guarded
			if test.readOnly {
				chain = ReadOnlyGuard("mail.test", true, chain)
			}
			chain = audit.AuditWrap("mail.test", "read", chain)
			result, err := chain(targetGuardAccountContext(entry), test.request)
			audit.InitAuditLog(false, "")
			if err != nil || result == nil || !result.IsError {
				t.Fatalf("chain result = %+v, error = %v", result, err)
			}
			data, readErr := os.ReadFile(auditPath)
			if readErr != nil {
				t.Fatalf("read audit: %v", readErr)
			}
			if lines := strings.Count(strings.TrimSpace(string(data)), "\n") + 1; lines != 1 {
				t.Fatalf("audit attempts = %d, data = %q", lines, data)
			}
		})
	}
	if handlerCalls.Load() != 0 || transportCalls.Load() != 0 {
		t.Fatalf("handler calls = %d, transport calls = %d", handlerCalls.Load(), transportCalls.Load())
	}
}
