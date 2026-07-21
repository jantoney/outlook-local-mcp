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

// TestOwnerCalendarListTiersUseExactRouteAndReferences verifies owner-primary
// list reads never use /me and expose signed provenance without modifying raw
// event objects.
func TestOwnerCalendarListTiersUseExactRouteAndReferences(t *testing.T) {
	paths := make(chan string, 3)
	client, server := newRealSDKGraphClient(t, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		paths <- request.URL.EscapedPath()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"value":[{"id":"event+1","subject":"Shared review","start":{"dateTime":"2026-03-12T09:00:00","timeZone":"UTC"},"end":{"dateTime":"2026-03-12T10:00:00","timeZone":"UTC"}}]}`))
	}))
	defer server.Close()
	ctx, codec := ownerCalendarContext(t, client, nil)
	handler := NewHandleListEvents(graph.RetryConfig{}, 30*time.Second, "UTC", "", &codec)

	for _, output := range []string{"text", "summary", "raw"} {
		request := calendarOwnerReadRequest(output)
		result, err := handler(ctx, request)
		if err != nil || result.IsError {
			t.Fatalf("list_events(%s) result = %+v, error = %v", output, result, err)
		}
		body := result.Content[0].(mcp.TextContent).Text
		switch output {
		case "text":
			if !strings.Contains(body, "Resource Ref: v1.") {
				t.Fatalf("text output missing signed reference: %s", body)
			}
		case "summary":
			var events []map[string]any
			if err := json.Unmarshal([]byte(body), &events); err != nil || len(events) != 1 {
				t.Fatalf("summary output = %s, error = %v", body, err)
			}
			assertOwnerEventReference(t, codec, events[0]["resource_ref"])
		case "raw":
			var envelope struct {
				Data       []map[string]any `json:"data"`
				Provenance []map[string]any `json:"provenance"`
			}
			if err := json.Unmarshal([]byte(body), &envelope); err != nil {
				t.Fatalf("raw output = %s, error = %v", body, err)
			}
			if len(envelope.Data) != 1 || len(envelope.Provenance) != 1 {
				t.Fatalf("raw envelope = %+v", envelope)
			}
			if _, modified := envelope.Data[0]["resource_ref"]; modified {
				t.Fatal("raw Graph event was modified with resource_ref")
			}
			assertOwnerEventReference(t, codec, envelope.Provenance[0]["resource_ref"])
		}
	}

	for range 3 {
		if path := <-paths; path != "/v1.0/users/owner%2Bcalendar%40example.com/calendarView" {
			t.Fatalf("owner list path = %q", path)
		}
	}
}

// TestOwnerCalendarSearchReturnsReference verifies search uses the same owner
// root and emits a target-bound summary reference.
func TestOwnerCalendarSearchReturnsReference(t *testing.T) {
	var path string
	client, server := newRealSDKGraphClient(t, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		path = request.URL.EscapedPath()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"value":[{"id":"event+1","subject":"Shared review","start":{"dateTime":"2026-03-12T09:00:00","timeZone":"UTC"},"end":{"dateTime":"2026-03-12T10:00:00","timeZone":"UTC"}}]}`))
	}))
	defer server.Close()
	ctx, codec := ownerCalendarContext(t, client, nil)
	handler := NewHandleSearchEvents(graph.RetryConfig{}, 30*time.Second, "UTC", "", &codec)
	request := calendarOwnerReadRequest("summary")
	result, err := handler(ctx, request)
	if err != nil || result.IsError {
		t.Fatalf("search_events result = %+v, error = %v", result, err)
	}
	var events []map[string]any
	body := result.Content[0].(mcp.TextContent).Text
	if err := json.Unmarshal([]byte(body), &events); err != nil || len(events) != 1 {
		t.Fatalf("summary output = %s, error = %v", body, err)
	}
	assertOwnerEventReference(t, codec, events[0]["resource_ref"])
	if path != "/v1.0/users/owner%2Bcalendar%40example.com/calendarView" {
		t.Fatalf("owner search path = %q", path)
	}
}

// TestOwnerCalendarGetConsumesReference verifies shared get derives the opaque
// event ID only from verified routed claims and returns renewed provenance.
func TestOwnerCalendarGetConsumesReference(t *testing.T) {
	var path string
	client, server := newRealSDKGraphClient(t, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		path = request.URL.EscapedPath()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"event+1","subject":"Shared review"}`))
	}))
	defer server.Close()
	baseCtx, codec := ownerCalendarContext(t, client, nil)
	routed, ok := graph.RoutedTargetFromContext(baseCtx)
	if !ok {
		t.Fatal("owner target missing from context")
	}
	claims := ownerEventClaims(routed.Target, "event+1")
	reference, err := codec.Sign(claims)
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}
	routed.Claims = &claims
	ctx := graph.WithRoutedTarget(baseCtx, routed)
	handler := NewHandleGetEvent(graph.RetryConfig{}, 30*time.Second, "UTC", "", &codec)
	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]any{"shared_resource": "team", "resource_ref": reference, "output": "summary"}
	result, err := handler(ctx, request)
	if err != nil || result.IsError {
		t.Fatalf("get_event result = %+v, error = %v", result, err)
	}
	var event map[string]any
	body := result.Content[0].(mcp.TextContent).Text
	if err := json.Unmarshal([]byte(body), &event); err != nil {
		t.Fatalf("summary output = %s, error = %v", body, err)
	}
	assertOwnerEventReference(t, codec, event["resource_ref"])
	if path != "/v1.0/users/owner%2Bcalendar%40example.com/events/event%2B1" {
		t.Fatalf("owner get path = %q", path)
	}
}

// TestOwnCalendarListPreservesMeRoute verifies omitting shared_resource keeps
// the existing recipient-view route and response shape without shared refs.
func TestOwnCalendarListPreservesMeRoute(t *testing.T) {
	var path string
	client, server := newRealSDKGraphClient(t, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		path = request.URL.EscapedPath()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"value":[{"id":"own-event","subject":"Own review"}]}`))
	}))
	defer server.Close()
	target, err := resource.NewOwnTarget(
		resource.AccountID("11111111-1111-4111-8111-111111111111"),
		resource.TargetFamilyCalendar, resource.MailActionPolicy{},
	)
	if err != nil {
		t.Fatalf("NewOwnTarget() error = %v", err)
	}
	root, err := graph.SelectUserRoot(client, target)
	if err != nil {
		t.Fatalf("SelectUserRoot() error = %v", err)
	}
	ctx := auth.WithGraphClient(context.Background(), client)
	ctx = graph.WithRoutedTarget(ctx, graph.RoutedTarget{Target: target, Root: root})
	handler := NewHandleListEvents(graph.RetryConfig{}, 30*time.Second, "UTC", "")
	request := calendarOwnerReadRequest("summary")
	delete(request.Params.Arguments.(map[string]any), "shared_resource")
	result, err := handler(ctx, request)
	if err != nil || result.IsError {
		t.Fatalf("own list result = %+v, error = %v", result, err)
	}
	if path != "/v1.0/me/calendarView" {
		t.Fatalf("own list path = %q", path)
	}
	if strings.Contains(result.Content[0].(mcp.TextContent).Text, "resource_ref") {
		t.Fatal("own response unexpectedly gained shared provenance")
	}
}

// calendarOwnerReadRequest creates one bounded calendar read request.
func calendarOwnerReadRequest(output string) mcp.CallToolRequest {
	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]any{
		"shared_resource": "team", "start_datetime": "2026-03-12T00:00:00Z",
		"end_datetime": "2026-03-13T00:00:00Z", "output": output,
	}
	return request
}

// ownerCalendarContext creates one authorized owner-primary routed target.
func ownerCalendarContext(t *testing.T, client *msgraphsdk.GraphServiceClient, claims *resource.ReferenceClaims) (context.Context, resource.ReferenceCodec) {
	t.Helper()
	key, err := resource.LoadOrCreateSigningKey(filepath.Join(t.TempDir(), "reference.key"))
	if err != nil {
		t.Fatalf("LoadOrCreateSigningKey() error = %v", err)
	}
	codec := resource.NewReferenceCodec(key)
	alias, err := resource.NewOwnerPrimaryCalendar(
		resource.ResourceID("22222222-2222-4222-8222-222222222222"),
		"team", "owner+calendar@example.com", resource.CalendarProfileRead,
	)
	if err != nil {
		t.Fatalf("NewOwnerPrimaryCalendar() error = %v", err)
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
	ctx = graph.WithRoutedTarget(ctx, graph.RoutedTarget{Target: target, Root: root, Claims: claims})
	return ctx, codec
}

// ownerEventClaims returns signed-reference claims for one owner event.
func ownerEventClaims(target resource.Target, eventID string) resource.ReferenceClaims {
	return resource.ReferenceClaims{
		AccountID: target.AccountID, ResourceID: target.ResourceID, ResourceKind: target.Kind,
		MailboxView: target.View, ItemKind: resource.ItemKindEvent,
		GraphIDChain: []resource.GraphID{{Kind: resource.ItemKindEvent, ID: eventID}},
	}
}

// assertOwnerEventReference verifies a response reference is owner-target bound.
func assertOwnerEventReference(t *testing.T, codec resource.ReferenceCodec, value any) {
	t.Helper()
	reference, ok := value.(string)
	if !ok || reference == "" {
		t.Fatalf("resource_ref = %#v", value)
	}
	claims, err := codec.Verify(reference)
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if claims.ResourceKind != resource.ResourceKindOwnerPrimaryCalendar || claims.MailboxView != resource.MailboxViewOwner || claims.GraphIDChain[0].ID != "event+1" {
		t.Fatalf("reference claims = %+v", claims)
	}
}
