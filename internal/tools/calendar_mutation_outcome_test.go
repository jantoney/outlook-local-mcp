package tools

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/desek/outlook-local-mcp/internal/auth"
	"github.com/desek/outlook-local-mcp/internal/graph"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/microsoftgraph/msgraph-sdk-go/models/odataerrors"
)

// TestCalendarMutationUncertaintyClassification verifies ambiguous transport,
// timeout, throttling, and server failures are distinct from definite 4xx.
func TestCalendarMutationUncertaintyClassification(t *testing.T) {
	statusError := func(status int) error {
		err := odataerrors.NewODataError()
		err.ResponseStatusCode = status
		return err
	}
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "transport", err: errors.New("connection reset"), want: true},
		{name: "timeout", err: context.DeadlineExceeded, want: true},
		{name: "throttled", err: statusError(http.StatusTooManyRequests), want: true},
		{name: "server", err: statusError(http.StatusServiceUnavailable), want: true},
		{name: "gateway", err: statusError(http.StatusGatewayTimeout), want: true},
		{name: "definite", err: statusError(http.StatusBadRequest), want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := calendarMutationOutcomeUncertain(test.err); got != test.want {
				t.Fatalf("calendarMutationOutcomeUncertain() = %t, want %t", got, test.want)
			}
		})
	}
}

// TestAmbiguousCalendarHandlersRequireReconciliation verifies each assigned
// one-attempt mutation gives action-specific, non-retry recovery guidance.
func TestAmbiguousCalendarHandlersRequireReconciliation(t *testing.T) {
	tests := []struct {
		name      string
		handler   func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error)
		arguments map[string]any
		want      string
	}{
		{name: "create", handler: HandleCreateEvent(graph.RetryConfig{}, time.Second, "UTC", ""), arguments: map[string]any{"subject": "Review", "start_datetime": "2026-08-01T09:00:00"}, want: "calendar around the requested time"},
		{name: "delete", handler: HandleDeleteEvent(graph.RetryConfig{}, time.Second), arguments: map[string]any{"event_id": "event-1"}, want: "whether the event remains"},
		{name: "respond", handler: HandleRespondEvent(graph.RetryConfig{}, time.Second), arguments: map[string]any{"event_id": "event-1", "response": "accept"}, want: "current response status"},
		{name: "cancel", handler: HandleCancelEvent(graph.RetryConfig{}, time.Second), arguments: map[string]any{"event_id": "event-1"}, want: "organizer calendar"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var calls int
			client, server := newRealSDKGraphClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls++
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Retry-After", "0")
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = w.Write([]byte(`{"error":{"code":"ServiceUnavailable","message":"ambiguous"}}`))
			}))
			defer server.Close()
			ctx := auth.WithGraphClient(context.Background(), client)
			request := mcp.CallToolRequest{}
			request.Params.Arguments = test.arguments
			result, err := test.handler(ctx, request)
			if err != nil || !result.IsError || calls != 1 {
				t.Fatalf("result = %+v, calls = %d, error = %v", result, calls, err)
			}
			text := result.Content[0].(mcp.TextContent).Text
			if !strings.Contains(text, "outcome is uncertain") || !strings.Contains(text, "do not retry blindly") || !strings.Contains(text, test.want) {
				t.Fatalf("ambiguous result lacks reconciliation guidance: %s", text)
			}
		})
	}
}
