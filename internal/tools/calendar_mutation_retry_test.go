package tools

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/desek/outlook-local-mcp/internal/graph"
	"github.com/desek/outlook-local-mcp/internal/resource"
	"github.com/mark3labs/mcp-go/mcp"
)

// TestMountedCalendarCreateAndDeleteAreAttemptedOnce verifies non-idempotent
// creates and destructive deletes never repeat after an ambiguous response.
func TestMountedCalendarCreateAndDeleteAreAttemptedOnce(t *testing.T) {
	tests := []struct {
		name    string
		method  string
		request map[string]any
		handler func(resource.ReferenceCodec) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error)
	}{
		{
			name: "create", method: http.MethodPost,
			request: map[string]any{"subject": "Review", "start_datetime": "2026-08-01T09:00:00", "shared_resource": "team-mount"},
			handler: func(codec resource.ReferenceCodec) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				return HandleCreateEvent(graph.RetryConfig{MaxRetries: 2, InitialBackoff: time.Nanosecond}, time.Second, "UTC", "", &codec)
			},
		},
		{
			name: "delete", method: http.MethodDelete,
			request: map[string]any{"shared_resource": "team-mount"},
			handler: func(resource.ReferenceCodec) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				return HandleDeleteEvent(graph.RetryConfig{MaxRetries: 2, InitialBackoff: time.Nanosecond}, time.Second)
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var mutationCalls int
			client, server := newRealSDKGraphClient(t, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if request.Method == http.MethodGet {
					_, _ = w.Write([]byte(`{"id":"event+1","attendees":[],"isOnlineMeeting":false}`))
					return
				}
				if request.Method == test.method {
					mutationCalls++
				}
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = w.Write([]byte(`{"error":{"code":"ServiceUnavailable","message":"ambiguous"}}`))
			}))
			defer server.Close()
			ctx, codec := mountedManageContext(t, client, func() error { return nil })
			request := mcp.CallToolRequest{}
			request.Params.Arguments = test.request
			result, err := test.handler(codec)(ctx, request)
			if err != nil || !result.IsError {
				t.Fatalf("result = %+v, error = %v", result, err)
			}
			if mutationCalls != 1 {
				t.Fatalf("mutation calls = %d, want exactly one", mutationCalls)
			}
		})
	}
}
