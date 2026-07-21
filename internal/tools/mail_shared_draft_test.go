package tools

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/desek/outlook-local-mcp/internal/graph"
	"github.com/desek/outlook-local-mcp/internal/resource"
	"github.com/mark3labs/mcp-go/mcp"
	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"
)

// TestSharedDraftCreatesUseExactOwnerRoutes verifies new, reply, and forward
// drafts never fall back from the configured shared mailbox owner view.
func TestSharedDraftCreatesUseExactOwnerRoutes(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		handler func(resource.ReferenceCodec) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error)
		kind    resource.ItemKind
	}{
		{"new", "/v1.0/users/shared%2Bops%40example.com/messages", func(codec resource.ReferenceCodec) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return NewHandleCreateDraft(graph.RetryConfig{}, time.Second, "", &codec)
		}, ""},
		{"reply", "/v1.0/users/shared%2Bops%40example.com/messages/source%2B1/createReply", func(codec resource.ReferenceCodec) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return NewHandleCreateReplyDraft(graph.RetryConfig{}, time.Second, "", &codec)
		}, resource.ItemKindMessage},
		{"forward", "/v1.0/users/shared%2Bops%40example.com/messages/source%2B1/createForward", func(codec resource.ReferenceCodec) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return NewHandleCreateForwardDraft(graph.RetryConfig{}, time.Second, "", &codec)
		}, resource.ItemKindMessage},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var paths []string
			client, server := newRealSDKGraphClient(t, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				paths = append(paths, request.Method+" "+request.URL.EscapedPath())
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"id":"draft+1","subject":"Shared draft","isDraft":true}`))
			}))
			defer server.Close()
			ctx, codec := sharedDraftContext(t, client, test.kind, "source+1", func() error { return nil })
			request := sharedDraftRequest()
			result, err := test.handler(codec)(ctx, request)
			if err != nil || result.IsError {
				t.Fatalf("result = %+v, error = %v", result, err)
			}
			if got := strings.Join(paths, "|"); got != "POST "+test.path {
				t.Fatalf("paths = %v, want POST %s", paths, test.path)
			}
			text := result.Content[0].(mcp.TextContent).Text
			if !strings.Contains(text, "Draft Ref: v1.") {
				t.Fatalf("confirmation lacks draft_ref: %s", text)
			}
		})
	}
}

// TestSharedDraftMutationsPreflightAndUseExactOwnerRoute verifies update and
// delete classify a draft before mutating the same immutable owner route.
func TestSharedDraftMutationsPreflightAndUseExactOwnerRoute(t *testing.T) {
	tests := []struct {
		name    string
		method  string
		handler func(resource.ReferenceCodec) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error)
		args    map[string]any
	}{
		{"update", http.MethodPatch, func(codec resource.ReferenceCodec) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return NewHandleUpdateDraft(graph.RetryConfig{}, time.Second, &codec)
		}, map[string]any{"subject": "Updated"}},
		{"delete", http.MethodDelete, func(resource.ReferenceCodec) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return NewHandleDeleteDraft(graph.RetryConfig{}, time.Second)
		}, map[string]any{}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var paths []string
			client, server := newRealSDKGraphClient(t, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				paths = append(paths, request.Method+" "+request.URL.EscapedPath())
				w.Header().Set("Content-Type", "application/json")
				if request.Method != http.MethodDelete {
					_, _ = w.Write([]byte(`{"id":"draft+1","subject":"Updated","isDraft":true}`))
				}
			}))
			defer server.Close()
			ctx, codec := sharedDraftContext(t, client, resource.ItemKindDraft, "draft+1", func() error { return nil })
			request := sharedDraftRequest()
			for key, value := range test.args {
				request.Params.Arguments.(map[string]any)[key] = value
			}
			result, err := test.handler(codec)(ctx, request)
			if err != nil || result.IsError {
				t.Fatalf("result = %+v, error = %v", result, err)
			}
			path := "/v1.0/users/shared%2Bops%40example.com/messages/draft%2B1"
			want := "GET " + path + "|" + test.method + " " + path
			if got := strings.Join(paths, "|"); got != want {
				t.Fatalf("paths = %v, want %s", paths, want)
			}
		})
	}
}

// TestSharedDraftMutationStopsAfterRevokedPreflight verifies a policy change
// between classification and mutation prevents the unstarted write.
func TestSharedDraftMutationStopsAfterRevokedPreflight(t *testing.T) {
	var methods []string
	client, server := newRealSDKGraphClient(t, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		methods = append(methods, request.Method)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"draft+1","isDraft":true}`))
	}))
	defer server.Close()
	ctx, codec := sharedDraftContext(t, client, resource.ItemKindDraft, "draft+1", func() error { return errors.New("draft permission was revoked") })
	request := sharedDraftRequest()
	request.Params.Arguments.(map[string]any)["subject"] = "Blocked"
	result, err := NewHandleUpdateDraft(graph.RetryConfig{}, time.Second, &codec)(ctx, request)
	if err != nil || !result.IsError || strings.Join(methods, "|") != http.MethodGet {
		t.Fatalf("result = %+v, error = %v, methods = %v", result, err, methods)
	}
}

// TestSharedDraftCreateStopsBeforePostWhenRevoked verifies authority is
// checked immediately before a non-idempotent shared draft creation begins.
func TestSharedDraftCreateStopsBeforePostWhenRevoked(t *testing.T) {
	var calls int
	client, server := newRealSDKGraphClient(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls++ }))
	defer server.Close()
	ctx, codec := sharedDraftContext(t, client, "", "", func() error { return errors.New("draft permission was revoked") })
	result, err := NewHandleCreateDraft(graph.RetryConfig{}, time.Second, "", &codec)(ctx, sharedDraftRequest())
	if err != nil || !result.IsError || calls != 0 {
		t.Fatalf("result = %+v, error = %v, Graph calls = %d", result, err, calls)
	}
}

// TestSharedDraftCreateDoesNotRetryAmbiguousFailure verifies a 5xx response
// cannot duplicate a non-idempotent create and produces recovery guidance.
func TestSharedDraftCreateDoesNotRetryAmbiguousFailure(t *testing.T) {
	var calls int
	client, server := newRealSDKGraphClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":{"code":"ServiceUnavailable","message":"ambiguous"}}`))
	}))
	defer server.Close()
	ctx, codec := sharedDraftContext(t, client, "", "", func() error { return nil })
	result, err := NewHandleCreateDraft(graph.RetryConfig{MaxRetries: 3}, time.Second, "", &codec)(ctx, sharedDraftRequest())
	if err != nil || !result.IsError || calls != 1 {
		t.Fatalf("result = %+v, error = %v, Graph calls = %d", result, err, calls)
	}
	if text := result.Content[0].(mcp.TextContent).Text; !strings.Contains(text, "outcome is uncertain") || !strings.Contains(text, "do not repeat") {
		t.Fatalf("ambiguous create guidance = %s", text)
	}
}

// TestSharedDraftMutationRejectsNonDraft verifies classification blocks the
// write when Graph explicitly reports isDraft=false.
func TestSharedDraftMutationRejectsNonDraft(t *testing.T) {
	var methods []string
	client, server := newRealSDKGraphClient(t, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		methods = append(methods, request.Method)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"message+1","isDraft":false}`))
	}))
	defer server.Close()
	ctx, codec := sharedDraftContext(t, client, resource.ItemKindDraft, "message+1", func() error { return nil })
	request := sharedDraftRequest()
	request.Params.Arguments.(map[string]any)["subject"] = "Blocked"
	result, err := NewHandleUpdateDraft(graph.RetryConfig{}, time.Second, &codec)(ctx, request)
	if err != nil || !result.IsError || strings.Join(methods, "|") != http.MethodGet {
		t.Fatalf("result = %+v, error = %v, methods = %v", result, err, methods)
	}
}

// TestSharedReplyReportsPartialSuccessAfterProvenanceFailure verifies the
// committed draft remains recoverable and the non-idempotent create is not
// presented as an ordinary success when required completion fails.
func TestSharedReplyReportsPartialSuccessAfterProvenanceFailure(t *testing.T) {
	var paths []string
	client, server := newRealSDKGraphClient(t, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		paths = append(paths, request.Method+" "+request.URL.EscapedPath())
		w.Header().Set("Content-Type", "application/json")
		if request.Method == http.MethodPatch {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":{"code":"InternalError","message":"completion failed"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":"draft+1","subject":"Reply"}`))
	}))
	defer server.Close()
	ctx, codec := sharedDraftContext(t, client, resource.ItemKindMessage, "source+1", func() error { return nil })
	result, err := NewHandleCreateReplyDraft(graph.RetryConfig{}, time.Second, "String {00020329-0000-0000-C000-000000000046} Name OutlookMCP", &codec)(ctx, sharedDraftRequest())
	if err != nil || result.IsError {
		t.Fatalf("result = %+v, error = %v", result, err)
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "PARTIAL SUCCESS") || !strings.Contains(text, "Draft Ref: v1.") || !strings.Contains(text, "Do not repeat") {
		t.Fatalf("partial confirmation = %s", text)
	}
	want := "POST /v1.0/users/shared%2Bops%40example.com/messages/source%2B1/createReply|PATCH /v1.0/users/shared%2Bops%40example.com/messages/draft%2B1"
	if got := strings.Join(paths, "|"); got != want {
		t.Fatalf("paths = %v, want %s", paths, want)
	}
}

// sharedDraftContext creates an immutable shared-mail owner route with draft
// policy and optional source or draft claims.
func sharedDraftContext(t *testing.T, client *msgraphsdk.GraphServiceClient, kind resource.ItemKind, itemID string, reauthorize func() error) (context.Context, resource.ReferenceCodec) {
	t.Helper()
	ctx, codec, target := sharedMailContext(t, client, nil)
	target.MailPolicy.Draft = true
	var claims *resource.ReferenceClaims
	if kind != "" {
		claims = &resource.ReferenceClaims{
			AccountID: target.AccountID, ResourceID: target.ResourceID, ResourceKind: target.Kind,
			MailboxView: target.View, ItemKind: kind, GraphIDChain: []resource.GraphID{{Kind: kind, ID: itemID}},
		}
	}
	routed, _ := graph.RoutedTargetFromContext(ctx)
	routed.Target, routed.Claims, routed.Reauthorize = target, claims, reauthorize
	return graph.WithRoutedTarget(ctx, routed), codec
}

// sharedDraftRequest creates a shared draft operation request whose item IDs
// are supplied only through verified routed claims.
func sharedDraftRequest() mcp.CallToolRequest {
	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]any{"shared_resource": "finance-mail"}
	return request
}
