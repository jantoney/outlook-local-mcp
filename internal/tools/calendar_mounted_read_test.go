package tools

import (
	"context"
	"encoding/json"
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

// TestMountedCalendarReadsUseConfiguredRecipientRoute verifies list, search,
// and get apply the selected mounted ID beneath /me and return mounted refs.
func TestMountedCalendarReadsUseConfiguredRecipientRoute(t *testing.T) {
	paths := make(chan string, 3)
	client, server := newRealSDKGraphClient(t, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		paths <- request.URL.EscapedPath()
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(request.URL.EscapedPath(), "/events/") {
			_, _ = w.Write([]byte(`{"id":"event+1","subject":"Mounted review"}`))
			return
		}
		_, _ = w.Write([]byte(`{"value":[{"id":"event+1","subject":"Mounted review"}]}`))
	}))
	defer server.Close()
	ctx, codec, target := mountedCalendarContext(t, client)

	listResult, err := NewHandleListEvents(graph.RetryConfig{}, 30*time.Second, "UTC", "", &codec)(ctx, calendarMountedReadRequest("summary"))
	if err != nil || listResult.IsError {
		t.Fatalf("list result = %+v, error = %v", listResult, err)
	}
	assertMountedSummaryReference(t, codec, listResult)

	searchResult, err := NewHandleSearchEvents(graph.RetryConfig{}, 30*time.Second, "UTC", "", &codec)(ctx, calendarMountedReadRequest("summary"))
	if err != nil || searchResult.IsError {
		t.Fatalf("search result = %+v, error = %v", searchResult, err)
	}
	assertMountedSummaryReference(t, codec, searchResult)

	claims := ownerEventClaims(target, "event+1")
	reference, err := codec.Sign(claims)
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}
	routed, _ := graph.RoutedTargetFromContext(ctx)
	routed.Claims = &claims
	getCtx := graph.WithRoutedTarget(ctx, routed)
	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]any{"shared_resource": "team-mount", "resource_ref": reference, "output": "summary"}
	getResult, err := NewHandleGetEvent(graph.RetryConfig{}, 30*time.Second, "UTC", "", &codec)(getCtx, request)
	if err != nil || getResult.IsError {
		t.Fatalf("get result = %+v, error = %v", getResult, err)
	}

	want := []string{
		"/v1.0/me/calendars/mounted+id/calendarView",
		"/v1.0/me/calendars/mounted+id/calendarView",
		"/v1.0/me/calendars/mounted+id/events/event+1",
	}
	for _, expected := range want {
		if path := <-paths; path != expected {
			t.Fatalf("mounted path = %q, want %q", path, expected)
		}
	}
}

// TestMountedCalendarMissingReturnsReselectionGuidance verifies a missing mount
// makes one exact request and never falls back to owner or default /me view.
func TestMountedCalendarMissingReturnsReselectionGuidance(t *testing.T) {
	var paths []string
	client, server := newRealSDKGraphClient(t, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		paths = append(paths, request.URL.EscapedPath())
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"code":"ErrorItemNotFound","message":"Calendar not found"}}`))
	}))
	defer server.Close()
	ctx, codec, _ := mountedCalendarContext(t, client)
	result, err := NewHandleListEvents(graph.RetryConfig{}, 30*time.Second, "UTC", "", &codec)(ctx, calendarMountedReadRequest("text"))
	if err != nil || !result.IsError {
		t.Fatalf("missing mount result = %+v, error = %v", result, err)
	}
	body := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(body, "reselect_calendar_alias") {
		t.Fatalf("missing mount error lacks reselection guidance: %s", body)
	}
	if len(paths) != 1 || paths[0] != "/v1.0/me/calendars/mounted+id/calendarView" {
		t.Fatalf("missing mount paths = %v", paths)
	}
}

// calendarMountedReadRequest creates one mounted shared collection request.
func calendarMountedReadRequest(output string) mcp.CallToolRequest {
	request := calendarOwnerReadRequest(output)
	request.Params.Arguments.(map[string]any)["shared_resource"] = "team-mount"
	return request
}

// mountedCalendarContext creates one authorized mounted recipient-view target.
func mountedCalendarContext(t *testing.T, client *msgraphsdk.GraphServiceClient) (context.Context, resource.ReferenceCodec, resource.Target) {
	t.Helper()
	key, err := resource.LoadOrCreateSigningKey(filepath.Join(t.TempDir(), "reference.key"))
	if err != nil {
		t.Fatalf("LoadOrCreateSigningKey() error = %v", err)
	}
	codec := resource.NewReferenceCodec(key)
	alias, err := resource.NewMountedCalendar(
		resource.ResourceID("55555555-5555-4555-8555-555555555555"),
		"team-mount", "owner@example.com", "mounted+id", resource.CalendarProfileRead,
	)
	if err != nil {
		t.Fatalf("NewMountedCalendar() error = %v", err)
	}
	target, err := resource.TargetFromCalendarAlias(
		resource.AccountID("11111111-1111-4111-8111-111111111111"), alias,
	)
	if err != nil {
		t.Fatalf("TargetFromCalendarAlias() error = %v", err)
	}
	root, err := graph.SelectUserRoot(client, target)
	if err != nil {
		t.Fatalf("SelectUserRoot() error = %v", err)
	}
	ctx := auth.WithGraphClient(context.Background(), client)
	ctx = graph.WithRoutedTarget(ctx, graph.RoutedTarget{Target: target, Root: root})
	return ctx, codec, target
}

// assertMountedSummaryReference verifies the first summary event is mounted.
func assertMountedSummaryReference(t *testing.T, codec resource.ReferenceCodec, result *mcp.CallToolResult) {
	t.Helper()
	var events []map[string]any
	body := result.Content[0].(mcp.TextContent).Text
	if err := json.Unmarshal([]byte(body), &events); err != nil || len(events) != 1 {
		t.Fatalf("mounted summary = %s, error = %v", body, err)
	}
	reference, _ := events[0]["resource_ref"].(string)
	claims, err := codec.Verify(reference)
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if claims.ResourceKind != resource.ResourceKindMountedCalendar || claims.MailboxView != resource.MailboxViewRecipient {
		t.Fatalf("mounted claims = %+v", claims)
	}
}
