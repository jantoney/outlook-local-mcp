package tools

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/desek/outlook-local-mcp/internal/graph"
	"github.com/desek/outlook-local-mcp/internal/resource"
	"github.com/mark3labs/mcp-go/mcp"
)

// TestMountedCalendarManageRetriesTheExactRoute verifies a transient write is
// retried without changing its authorized mounted calendar or event path.
func TestMountedCalendarManageRetriesTheExactRoute(t *testing.T) {
	var paths []string
	var patchCalls int
	client, server := newRealSDKGraphClient(t, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		paths = append(paths, request.Method+" "+request.URL.EscapedPath())
		w.Header().Set("Content-Type", "application/json")
		if request.Method == http.MethodPatch {
			patchCalls++
			if patchCalls == 1 {
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = w.Write([]byte(`{"error":{"code":"ServiceUnavailable","message":"retry"}}`))
				return
			}
		}
		_, _ = w.Write([]byte(`{"id":"event+1","subject":"Updated","attendees":[],"isOnlineMeeting":false}`))
	}))
	defer server.Close()
	ctx, codec := mountedManageContext(t, client, func() error { return nil })
	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]any{"subject": "Updated", "shared_resource": "team-mount"}
	retry := graph.RetryConfig{MaxRetries: 1, InitialBackoff: time.Nanosecond, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	result, err := HandleUpdateEvent(retry, 30*time.Second, "UTC", &codec)(ctx, request)
	if err != nil || result.IsError {
		t.Fatalf("result = %+v, error = %v", result, err)
	}
	want := []string{
		"GET /v1.0/me/calendars/mounted+id/events/event+1",
		"PATCH /v1.0/me/calendars/mounted+id/events/event+1",
		"PATCH /v1.0/me/calendars/mounted+id/events/event+1",
	}
	if strings.Join(paths, "|") != strings.Join(want, "|") {
		t.Fatalf("paths = %v, want %v", paths, want)
	}
}

// TestMountedCalendarManageRejectsMeetingsBeforeMutation verifies an event
// with attendees is read for classification but is never patched or deleted.
func TestMountedCalendarManageRejectsMeetingsBeforeMutation(t *testing.T) {
	var methods []string
	client, server := newRealSDKGraphClient(t, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		methods = append(methods, request.Method)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"event+1","attendees":[{"emailAddress":{"address":"person@example.com"},"type":"required"}],"isOnlineMeeting":false}`))
	}))
	defer server.Close()
	ctx, codec := mountedManageContext(t, client, func() error { return nil })
	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]any{"subject": "Blocked", "shared_resource": "team-mount"}
	result, err := HandleUpdateEvent(graph.RetryConfig{}, 30*time.Second, "UTC", &codec)(ctx, request)
	if err != nil || !result.IsError {
		t.Fatalf("result = %+v, error = %v", result, err)
	}
	if len(methods) != 1 || methods[0] != http.MethodGet {
		t.Fatalf("methods = %v, want one GET", methods)
	}
	if text := result.Content[0].(mcp.TextContent).Text; !strings.Contains(text, "without attendees") {
		t.Fatalf("error lacks meeting guidance: %s", text)
	}
}

// TestMountedCalendarManageRejectsOnlineMeetingInput verifies mounted create
// and update cannot enable durable online-meeting capability.
func TestMountedCalendarManageRejectsOnlineMeetingInput(t *testing.T) {
	var calls int
	client, server := newRealSDKGraphClient(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls++ }))
	defer server.Close()
	ctx, codec := mountedManageContext(t, client, func() error { return nil })
	tests := []struct {
		name    string
		handler func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error)
		args    map[string]any
	}{
		{"create", HandleCreateEvent(graph.RetryConfig{}, time.Second, "UTC", "", &codec), map[string]any{"subject": "Online", "start_datetime": "2026-08-01T09:00:00", "is_online_meeting": true}},
		{"update", HandleUpdateEvent(graph.RetryConfig{}, time.Second, "UTC", &codec), map[string]any{"subject": "Online", "is_online_meeting": true}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := mcp.CallToolRequest{}
			request.Params.Arguments = test.args
			result, err := test.handler(ctx, request)
			if err != nil || !result.IsError {
				t.Fatalf("result = %+v, error = %v", result, err)
			}
			if text := result.Content[0].(mcp.TextContent).Text; !strings.Contains(text, "online meetings") {
				t.Fatalf("error lacks online-meeting guidance: %s", text)
			}
		})
	}
	if calls != 0 {
		t.Fatalf("Graph calls = %d, want zero", calls)
	}
}

// TestMountedCalendarManageRejectsExistingOnlineMeetings verifies every
// follow-up mutation stops after the classification GET.
func TestMountedCalendarManageRejectsExistingOnlineMeetings(t *testing.T) {
	tests := []struct {
		name    string
		handler func(resource.ReferenceCodec) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error)
		args    map[string]any
	}{
		{"update", func(codec resource.ReferenceCodec) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return HandleUpdateEvent(graph.RetryConfig{}, time.Second, "UTC", &codec)
		}, map[string]any{"subject": "Blocked"}},
		{"reschedule", func(codec resource.ReferenceCodec) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return HandleRescheduleEvent(graph.RetryConfig{}, time.Second, "UTC", &codec)
		}, map[string]any{"new_start_datetime": "2026-08-02T09:00:00"}},
		{"delete", func(resource.ReferenceCodec) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return HandleDeleteEvent(graph.RetryConfig{}, time.Second)
		}, map[string]any{}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var methods []string
			client, server := newRealSDKGraphClient(t, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				methods = append(methods, request.Method)
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"id":"event+1","attendees":[],"isOnlineMeeting":true,"start":{"dateTime":"2026-08-01T09:00:00","timeZone":"UTC"},"end":{"dateTime":"2026-08-01T10:00:00","timeZone":"UTC"}}`))
			}))
			defer server.Close()
			ctx, codec := mountedManageContext(t, client, func() error { return nil })
			request := mcp.CallToolRequest{}
			request.Params.Arguments = test.args
			result, err := test.handler(codec)(ctx, request)
			if err != nil || !result.IsError {
				t.Fatalf("result = %+v, error = %v", result, err)
			}
			if len(methods) != 1 || methods[0] != http.MethodGet {
				t.Fatalf("methods = %v, want one GET", methods)
			}
			if text := result.Content[0].(mcp.TextContent).Text; !strings.Contains(text, "online meetings") {
				t.Fatalf("error lacks online-meeting guidance: %s", text)
			}
		})
	}
}

// TestMountedCalendarManageRequiresOnlineClassification verifies a missing
// isOnlineMeeting field fails closed after preflight.
func TestMountedCalendarManageRequiresOnlineClassification(t *testing.T) {
	var methods []string
	client, server := newRealSDKGraphClient(t, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		methods = append(methods, request.Method)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"event+1","attendees":[]}`))
	}))
	defer server.Close()
	ctx, codec := mountedManageContext(t, client, func() error { return nil })
	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]any{"subject": "Blocked"}
	result, err := HandleUpdateEvent(graph.RetryConfig{}, time.Second, "UTC", &codec)(ctx, request)
	if err != nil || !result.IsError {
		t.Fatalf("result = %+v, error = %v", result, err)
	}
	if len(methods) != 1 || methods[0] != http.MethodGet {
		t.Fatalf("methods = %v, want one GET", methods)
	}
	if text := result.Content[0].(mcp.TextContent).Text; !strings.Contains(text, "classification is unavailable") {
		t.Fatalf("error lacks classification guidance: %s", text)
	}
}

// TestMountedCalendarManageRevalidatesBeforeMutation verifies authority
// revoked after preflight prevents the independently committable write.
func TestMountedCalendarManageRevalidatesBeforeMutation(t *testing.T) {
	var methods []string
	client, server := newRealSDKGraphClient(t, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		methods = append(methods, request.Method)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"event+1","attendees":[],"isOnlineMeeting":false}`))
	}))
	defer server.Close()
	ctx, codec := mountedManageContext(t, client, func() error { return errors.New("manage permission was revoked") })
	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]any{"subject": "Blocked", "shared_resource": "team-mount"}
	result, err := HandleUpdateEvent(graph.RetryConfig{}, 30*time.Second, "UTC", &codec)(ctx, request)
	if err != nil || !result.IsError {
		t.Fatalf("result = %+v, error = %v", result, err)
	}
	if len(methods) != 1 || methods[0] != http.MethodGet {
		t.Fatalf("methods = %v, want one GET", methods)
	}
	if text := result.Content[0].(mcp.TextContent).Text; !strings.Contains(text, "revoked") {
		t.Fatalf("error lacks actionable revocation reason: %s", text)
	}
}

// TestSharedMeetingActionsRejectRoutingInputs verifies undeclared shared
// inputs cannot silently mutate the signed-in account's own meeting.
func TestSharedMeetingActionsRejectRoutingInputs(t *testing.T) {
	var calls int
	client, server := newRealSDKGraphClient(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls++ }))
	defer server.Close()
	ctx, _ := mountedManageContext(t, client, func() error { return nil })
	tests := []struct {
		name    string
		handler func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error)
		args    map[string]any
	}{
		{"respond", HandleRespondEvent(graph.RetryConfig{}, time.Second), map[string]any{"shared_resource": "team-mount", "event_id": "event+1", "response": "accept"}},
		{"cancel", HandleCancelEvent(graph.RetryConfig{}, time.Second), map[string]any{"resource_ref": "v1.reference", "event_id": "event+1"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := mcp.CallToolRequest{}
			request.Params.Arguments = test.args
			result, err := test.handler(ctx, request)
			if err != nil || !result.IsError {
				t.Fatalf("result = %+v, error = %v", result, err)
			}
			if text := result.Content[0].(mcp.TextContent).Text; !strings.Contains(text, "not supported") {
				t.Fatalf("error lacks shared meeting boundary: %s", text)
			}
		})
	}
	if calls != 0 {
		t.Fatalf("Graph calls = %d, want zero", calls)
	}
}
