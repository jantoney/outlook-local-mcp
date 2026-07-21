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
	"github.com/desek/outlook-local-mcp/internal/resource"
	"github.com/mark3labs/mcp-go/mcp"
	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"
)

// TestMountedCalendarManageUsesRecipientRoute verifies every supported
// non-meeting write remains beneath the configured recipient-view calendar.
func TestMountedCalendarManageUsesRecipientRoute(t *testing.T) {
	tests := []struct {
		name      string
		request   map[string]any
		wantCalls []string
		handler   func(resource.ReferenceCodec) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error)
	}{
		{
			name: "create", request: map[string]any{"subject": "Review", "start_datetime": "2026-08-01T09:00:00", "shared_resource": "team-mount"},
			wantCalls: []string{"POST /v1.0/me/calendars/mounted+id/events"},
			handler: func(codec resource.ReferenceCodec) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				return HandleCreateEvent(graph.RetryConfig{}, 30*time.Second, "UTC", "", &codec)
			},
		},
		{
			name: "update", request: map[string]any{"subject": "Updated", "shared_resource": "team-mount"},
			wantCalls: []string{"GET /v1.0/me/calendars/mounted+id/events/event+1", "PATCH /v1.0/me/calendars/mounted+id/events/event+1"},
			handler: func(codec resource.ReferenceCodec) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				return HandleUpdateEvent(graph.RetryConfig{}, 30*time.Second, "UTC", &codec)
			},
		},
		{
			name: "delete", request: map[string]any{"shared_resource": "team-mount"},
			wantCalls: []string{"GET /v1.0/me/calendars/mounted+id/events/event+1", "DELETE /v1.0/me/calendars/mounted+id/events/event+1"},
			handler: func(resource.ReferenceCodec) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				return HandleDeleteEvent(graph.RetryConfig{}, 30*time.Second)
			},
		},
		{
			name: "reschedule", request: map[string]any{"new_start_datetime": "2026-08-02T10:00:00", "shared_resource": "team-mount"},
			wantCalls: []string{"GET /v1.0/me/calendars/mounted+id/events/event+1", "PATCH /v1.0/me/calendars/mounted+id/events/event+1"},
			handler: func(codec resource.ReferenceCodec) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				return HandleRescheduleEvent(graph.RetryConfig{}, 30*time.Second, "UTC", &codec)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var calls []string
			client, server := newRealSDKGraphClient(t, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				calls = append(calls, request.Method+" "+request.URL.EscapedPath())
				w.Header().Set("Content-Type", "application/json")
				if request.Method == http.MethodDelete {
					w.WriteHeader(http.StatusNoContent)
					return
				}
				_, _ = w.Write([]byte(`{"id":"event+1","subject":"Review","start":{"dateTime":"2026-08-01T09:00:00","timeZone":"UTC"},"end":{"dateTime":"2026-08-01T10:00:00","timeZone":"UTC"},"attendees":[],"isOnlineMeeting":false}`))
			}))
			defer server.Close()
			ctx, codec := mountedManageContext(t, client, func() error { return nil })
			request := mcp.CallToolRequest{}
			request.Params.Arguments = test.request
			result, err := test.handler(codec)(ctx, request)
			if err != nil || result.IsError {
				t.Fatalf("result = %+v, error = %v", result, err)
			}
			if strings.Join(calls, "|") != strings.Join(test.wantCalls, "|") {
				t.Fatalf("calls = %v, want %v", calls, test.wantCalls)
			}
			text := result.Content[0].(mcp.TextContent).Text
			if test.name != "delete" && !strings.Contains(text, "Resource Ref: v1.") {
				t.Fatalf("confirmation lacks renewed provenance: %s", text)
			}
		})
	}
}

// mountedManageContext creates one authorized mounted manage target whose
// follow-up event ID is supplied only through verified reference claims.
func mountedManageContext(t *testing.T, client *msgraphsdk.GraphServiceClient, reauthorize func() error) (context.Context, resource.ReferenceCodec) {
	t.Helper()
	key, err := resource.LoadOrCreateSigningKey(filepath.Join(t.TempDir(), "reference.key"))
	if err != nil {
		t.Fatalf("LoadOrCreateSigningKey() error = %v", err)
	}
	codec := resource.NewReferenceCodec(key)
	alias, err := resource.NewMountedCalendar(
		resource.ResourceID("55555555-5555-4555-8555-555555555555"),
		"team-mount", "owner@example.com", "mounted+id", resource.CalendarProfileManage,
	)
	if err != nil {
		t.Fatalf("NewMountedCalendar() error = %v", err)
	}
	target, err := resource.TargetFromCalendarAlias(resource.AccountID("11111111-1111-4111-8111-111111111111"), alias)
	if err != nil {
		t.Fatalf("TargetFromCalendarAlias() error = %v", err)
	}
	root, err := graph.SelectUserRoot(client, target)
	if err != nil {
		t.Fatalf("SelectUserRoot() error = %v", err)
	}
	claims := ownerEventClaims(target, "event+1")
	ctx := auth.WithGraphClient(context.Background(), client)
	ctx = graph.WithRoutedTarget(ctx, graph.RoutedTarget{Target: target, Root: root, Claims: &claims, Reauthorize: reauthorize})
	return ctx, codec
}
