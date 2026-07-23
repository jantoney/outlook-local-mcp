package server

import (
	"context"
	"net/http"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/desek/outlook-local-mcp/internal/auth"
	"github.com/desek/outlook-local-mcp/internal/graph"
	"github.com/desek/outlook-local-mcp/internal/resource"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"
)

// TestMountedCalendarGetGuardRejectsOtherViews verifies owner-primary, own,
// and other mounted references fail before the handler or Graph transport.
func TestMountedCalendarGetGuardRejectsOtherViews(t *testing.T) {
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

	for _, source := range []string{"owner", "own", "another mount"} {
		t.Run(source, func(t *testing.T) {
			registry, entry, target, codec := mountedGuardFixture(t, client, auth.TokenTenantOrganizational)
			claims := mountedForeignClaims(t, source, target)
			reference, err := codec.Sign(claims)
			if err != nil {
				t.Fatalf("Sign() error = %v", err)
			}
			request := mcp.CallToolRequest{}
			request.Params.Arguments = map[string]any{"shared_resource": "team-mount", "resource_ref": reference}
			result, err := ResolvedTargetGuard(registry, calendarSharedGetGuard(), &codec, inner)(ownerGuardAccountContext(entry), request)
			if err != nil || !result.IsError {
				t.Fatalf("guard result = %+v, error = %v", result, err)
			}
		})
	}
	if handlerCalls.Load() != 0 || transportCalls.Load() != 0 {
		t.Fatalf("handler calls = %d, transport calls = %d; want zero", handlerCalls.Load(), transportCalls.Load())
	}
}

// TestMountedCalendarReadCompatibility verifies validated personal and
// organizational contexts are accepted while unknown fails before handlers.
func TestMountedCalendarReadCompatibility(t *testing.T) {
	client, server := newServerTestGraphClient(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer server.Close()
	var handlerCalls atomic.Int32
	inner := mcpserver.ToolHandlerFunc(func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		handlerCalls.Add(1)
		return mcp.NewToolResultText("ok"), nil
	})
	for _, tenant := range []auth.TokenTenantContext{auth.TokenTenantPersonal, auth.TokenTenantOrganizational, auth.TokenTenantUnknown} {
		registry, entry, _, codec := mountedGuardFixture(t, client, tenant)
		request := mcp.CallToolRequest{}
		request.Params.Arguments = map[string]any{"shared_resource": "team-mount"}
		result, err := ResolvedTargetGuard(registry, calendarSharedReadGuard(), &codec, inner)(ownerGuardAccountContext(entry), request)
		if err != nil {
			t.Fatalf("%s guard error = %v", tenant, err)
		}
		if tenant == auth.TokenTenantUnknown && !result.IsError {
			t.Fatal("unknown mounted read was accepted")
		}
		if tenant != auth.TokenTenantUnknown && result.IsError {
			t.Fatalf("%s mounted read result = %+v", tenant, result)
		}
	}
	if handlerCalls.Load() != 2 {
		t.Fatalf("handler calls = %d, want 2", handlerCalls.Load())
	}
}

// TestMountedCalendarManageGuardFailsClosed verifies compatibility, profile,
// provenance, and raw-ID failures stop before the handler and transport.
func TestMountedCalendarManageGuardFailsClosed(t *testing.T) {
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
		name    string
		tenant  auth.TokenTenantContext
		profile resource.CalendarProfile
		args    map[string]any
	}{
		{"personal tenant", auth.TokenTenantPersonal, resource.CalendarProfileManage, map[string]any{"shared_resource": "team-mount"}},
		{"unknown tenant", auth.TokenTenantUnknown, resource.CalendarProfileManage, map[string]any{"shared_resource": "team-mount"}},
		{"read profile", auth.TokenTenantOrganizational, resource.CalendarProfileRead, map[string]any{"shared_resource": "team-mount"}},
		{"missing reference", auth.TokenTenantOrganizational, resource.CalendarProfileManage, map[string]any{"shared_resource": "team-mount"}},
		{"raw ID", auth.TokenTenantOrganizational, resource.CalendarProfileManage, map[string]any{"shared_resource": "team-mount", "event_id": "event-1"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			registry, entry, target, codec := mountedGuardFixtureWithProfile(t, client, test.tenant, test.profile)
			if test.name != "missing reference" && test.name != "raw ID" {
				claims := guardedOwnerEventClaims(target)
				reference, err := codec.Sign(claims)
				if err != nil {
					t.Fatalf("Sign() error = %v", err)
				}
				test.args["resource_ref"] = reference
			}
			result, err := ResolvedTargetGuard(registry, calendarMountedManageEventGuard(), &codec, inner)(ownerGuardAccountContext(entry), requestWithArguments(test.args))
			if err != nil || !result.IsError {
				t.Fatalf("guard result = %+v, error = %v", result, err)
			}
		})
	}
	if handlerCalls.Load() != 0 || transportCalls.Load() != 0 {
		t.Fatalf("handler calls = %d, transport calls = %d; want zero", handlerCalls.Load(), transportCalls.Load())
	}
}

// TestMountedCalendarManageReauthorizationReadsCurrentPolicy verifies the
// middleware callback catches a profile downgrade after initial resolution.
func TestMountedCalendarManageReauthorizationReadsCurrentPolicy(t *testing.T) {
	client, server := newServerTestGraphClient(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer server.Close()
	registry, entry, target, codec := mountedGuardFixtureWithProfile(t, client, auth.TokenTenantOrganizational, resource.CalendarProfileManage)
	claims := guardedOwnerEventClaims(target)
	reference, err := codec.Sign(claims)
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}
	inner := mcpserver.ToolHandlerFunc(func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := registry.Update(entry.Label, func(current *auth.AccountEntry) {
			current.CalendarAliases[0].Profile = resource.CalendarProfileRead
		}); err != nil {
			t.Fatalf("registry.Update() error = %v", err)
		}
		routed, ok := graph.RoutedTargetFromContext(ctx)
		if !ok {
			t.Fatal("routed target missing")
		}
		if err := routed.Revalidate(); err == nil || !strings.Contains(err.Error(), "authority changed") {
			t.Fatalf("Revalidate() error = %v", err)
		}
		return mcp.NewToolResultText("checked"), nil
	})
	args := map[string]any{"shared_resource": "team-mount", "resource_ref": reference}
	result, err := ResolvedTargetGuard(registry, calendarMountedManageEventGuard(), &codec, inner)(ownerGuardAccountContext(entry), requestWithArguments(args))
	if err != nil || result.IsError {
		t.Fatalf("guard result = %+v, error = %v", result, err)
	}
}

// TestMountedCalendarManageReauthorizationDetectsClientSwap verifies a new
// connection cannot inherit authority captured for the prior Graph client.
func TestMountedCalendarManageReauthorizationDetectsClientSwap(t *testing.T) {
	client, server := newServerTestGraphClient(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer server.Close()
	replacement, replacementServer := newServerTestGraphClient(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer replacementServer.Close()
	registry, entry, target, codec := mountedGuardFixtureWithProfile(t, client, auth.TokenTenantOrganizational, resource.CalendarProfileManage)
	claims := guardedOwnerEventClaims(target)
	reference, err := codec.Sign(claims)
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}
	inner := mcpserver.ToolHandlerFunc(func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := registry.Update(entry.Label, func(current *auth.AccountEntry) { current.Client = replacement }); err != nil {
			t.Fatalf("registry.Update() error = %v", err)
		}
		routed, ok := graph.RoutedTargetFromContext(ctx)
		if !ok {
			t.Fatal("routed target missing")
		}
		if err := routed.Revalidate(); err == nil || !strings.Contains(err.Error(), "connection changed") {
			t.Fatalf("Revalidate() error = %v", err)
		}
		return mcp.NewToolResultText("checked"), nil
	})
	args := map[string]any{"shared_resource": "team-mount", "resource_ref": reference}
	result, err := ResolvedTargetGuard(registry, calendarMountedManageEventGuard(), &codec, inner)(ownerGuardAccountContext(entry), requestWithArguments(args))
	if err != nil || result.IsError {
		t.Fatalf("guard result = %+v, error = %v", result, err)
	}
}

// mountedGuardFixture creates one read-enabled mounted alias and signer.
func mountedGuardFixture(t *testing.T, client *msgraphsdk.GraphServiceClient, tenant auth.TokenTenantContext) (*auth.AccountRegistry, *auth.AccountEntry, resource.Target, resource.ReferenceCodec) {
	return mountedGuardFixtureWithProfile(t, client, tenant, resource.CalendarProfileRead)
}

// mountedGuardFixtureWithProfile creates one mounted alias with an exact
// profile for read and manage authorization tests.
func mountedGuardFixtureWithProfile(t *testing.T, client *msgraphsdk.GraphServiceClient, tenant auth.TokenTenantContext, profile resource.CalendarProfile) (*auth.AccountRegistry, *auth.AccountEntry, resource.Target, resource.ReferenceCodec) {
	t.Helper()
	key, err := resource.LoadOrCreateSigningKey(filepath.Join(t.TempDir(), "reference.key"))
	if err != nil {
		t.Fatalf("LoadOrCreateSigningKey() error = %v", err)
	}
	alias, err := resource.NewMountedCalendar(
		resource.ResourceID("55555555-5555-4555-8555-555555555555"),
		"team-mount", "owner@example.com", "mounted-id", profile,
	)
	if err != nil {
		t.Fatalf("NewMountedCalendar() error = %v", err)
	}
	alias.Validation = resource.Validation{Status: resource.ValidationAvailable}
	entry := &auth.AccountEntry{
		AccountID: auth.AccountID("11111111-1111-4111-8111-111111111111"),
		Label:     "work", Client: client, Authenticated: true,
		TokenTenantContext: tenant, CalendarAliases: []resource.CalendarAlias{alias},
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

// requestWithArguments constructs one MCP request for middleware tests.
func requestWithArguments(arguments map[string]any) mcp.CallToolRequest {
	request := mcp.CallToolRequest{}
	request.Params.Arguments = arguments
	return request
}

// mountedForeignClaims creates valid event provenance from a different view.
func mountedForeignClaims(t *testing.T, source string, target resource.Target) resource.ReferenceClaims {
	t.Helper()
	claims := guardedOwnerEventClaims(target)
	switch source {
	case "owner":
		claims.ResourceID = resource.ResourceID("22222222-2222-4222-8222-222222222222")
		claims.ResourceKind = resource.ResourceKindOwnerPrimaryCalendar
		claims.MailboxView = resource.MailboxViewOwner
	case "own":
		claims.ResourceID = resource.OwnCalendarResourceID
		claims.ResourceKind = resource.ResourceKindOwnCalendar
		claims.MailboxView = resource.MailboxViewRecipient
	case "another mount":
		claims.ResourceID = resource.ResourceID("66666666-6666-4666-8666-666666666666")
	default:
		t.Fatalf("unknown source %q", source)
	}
	return claims
}
