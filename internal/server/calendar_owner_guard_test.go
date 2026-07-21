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

// TestOwnerCalendarGetGuardRejectsInvalidProvenanceBeforeGraph verifies every
// target-bound reference mismatch and alias-plus-raw-ID attempt fails before
// the calendar handler or transport can run.
func TestOwnerCalendarGetGuardRejectsInvalidProvenanceBeforeGraph(t *testing.T) {
	var handlerCalls atomic.Int32
	var transportCalls atomic.Int32
	client, server := newServerTestGraphClient(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		transportCalls.Add(1)
	}))
	defer server.Close()
	inner := mcpserver.ToolHandlerFunc(func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		handlerCalls.Add(1)
		return mcp.NewToolResultText("unexpected"), nil
	})

	tests := []struct {
		name     string
		claims   func(resource.Target) resource.ReferenceClaims
		tamper   bool
		removed  bool
		rawEvent bool
	}{
		{name: "cross account", claims: func(target resource.Target) resource.ReferenceClaims {
			claims := guardedOwnerEventClaims(target)
			claims.AccountID = resource.AccountID("33333333-3333-4333-8333-333333333333")
			return claims
		}},
		{name: "cross resource", claims: func(target resource.Target) resource.ReferenceClaims {
			claims := guardedOwnerEventClaims(target)
			claims.ResourceID = resource.ResourceID("44444444-4444-4444-8444-444444444444")
			return claims
		}},
		{name: "cross view", claims: func(target resource.Target) resource.ReferenceClaims {
			claims := guardedOwnerEventClaims(target)
			claims.ResourceID = resource.OwnCalendarResourceID
			claims.ResourceKind = resource.ResourceKindOwnCalendar
			claims.MailboxView = resource.MailboxViewRecipient
			return claims
		}},
		{name: "wrong kind", claims: func(target resource.Target) resource.ReferenceClaims {
			claims := guardedOwnerEventClaims(target)
			claims.ResourceKind = resource.ResourceKindMailbox
			return claims
		}},
		{name: "tampered", claims: guardedOwnerEventClaims, tamper: true},
		{name: "removed target", claims: guardedOwnerEventClaims, removed: true},
		{name: "raw ID plus alias", claims: guardedOwnerEventClaims, rawEvent: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			registry, entry, target, codec := ownerGuardFixture(t, client)
			reference, err := codec.Sign(test.claims(target))
			if err != nil {
				t.Fatalf("Sign() error = %v", err)
			}
			if test.tamper {
				reference += "tampered"
			}
			if test.removed {
				if err := registry.Update(entry.Label, func(current *auth.AccountEntry) { current.CalendarAliases = nil }); err != nil {
					t.Fatalf("registry.Update() error = %v", err)
				}
			}
			request := mcp.CallToolRequest{}
			request.Params.Arguments = map[string]any{"shared_resource": "team", "resource_ref": reference}
			if test.rawEvent {
				request.Params.Arguments.(map[string]any)["event_id"] = "event-1"
			}
			guarded := ResolvedTargetGuard(registry, calendarSharedGetGuard(), &codec, inner)
			result, err := guarded(ownerGuardAccountContext(entry), request)
			if err != nil || !result.IsError {
				t.Fatalf("guard result = %+v, error = %v", result, err)
			}
		})
	}
	if handlerCalls.Load() != 0 || transportCalls.Load() != 0 {
		t.Fatalf("handler calls = %d, transport calls = %d; want zero", handlerCalls.Load(), transportCalls.Load())
	}
}

// ownerGuardFixture creates one connected organizational owner-primary alias.
func ownerGuardFixture(t *testing.T, client *msgraphsdk.GraphServiceClient) (*auth.AccountRegistry, *auth.AccountEntry, resource.Target, resource.ReferenceCodec) {
	t.Helper()
	key, err := resource.LoadOrCreateSigningKey(filepath.Join(t.TempDir(), "reference.key"))
	if err != nil {
		t.Fatalf("LoadOrCreateSigningKey() error = %v", err)
	}
	alias, err := resource.NewOwnerPrimaryCalendar(
		resource.ResourceID("22222222-2222-4222-8222-222222222222"),
		"team", "owner@example.com", resource.CalendarProfileRead,
	)
	if err != nil {
		t.Fatalf("NewOwnerPrimaryCalendar() error = %v", err)
	}
	entry := &auth.AccountEntry{
		AccountID: auth.AccountID("11111111-1111-4111-8111-111111111111"),
		Label:     "work", Client: client, Authenticated: true,
		TokenTenantContext: auth.TokenTenantOrganizational,
		CalendarAliases:    []resource.CalendarAlias{alias},
	}
	registry := auth.NewAccountRegistry()
	if err := registry.Add(entry); err != nil {
		t.Fatalf("registry.Add() error = %v", err)
	}
	target, err := resource.TargetFromCalendarAlias(entry.AccountID, alias)
	if err != nil {
		t.Fatalf("TargetFromCalendarAlias() error = %v", err)
	}
	return registry, entry, target, resource.NewReferenceCodec(key)
}

// guardedOwnerEventClaims returns valid claims for the fixture target.
func guardedOwnerEventClaims(target resource.Target) resource.ReferenceClaims {
	return resource.ReferenceClaims{
		AccountID: target.AccountID, ResourceID: target.ResourceID,
		ResourceKind: target.Kind, MailboxView: target.View,
		ItemKind:     resource.ItemKindEvent,
		GraphIDChain: []resource.GraphID{{Kind: resource.ItemKindEvent, ID: "event-1"}},
	}
}

// ownerGuardAccountContext selects the fixture account for target middleware.
func ownerGuardAccountContext(entry *auth.AccountEntry) context.Context {
	return auth.WithAccountInfo(context.Background(), auth.AccountInfo{
		AccountID: entry.AccountID, Label: entry.Label,
	})
}
