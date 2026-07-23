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
	"github.com/desek/outlook-local-mcp/internal/resource"
	"github.com/mark3labs/mcp-go/mcp"
)

// TestRemoveAttachmentUsesExactOwnRoute verifies the own-mail handler checks
// draft state before deleting exactly the requested attachment.
func TestRemoveAttachmentUsesExactOwnRoute(t *testing.T) {
	var calls []string
	client, server := newRealSDKGraphClient(t, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		calls = append(calls, request.Method+" "+request.URL.EscapedPath())
		w.Header().Set("Content-Type", "application/json")
		if request.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"id":"draft+1","isDraft":true}`))
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	ctx := auth.WithGraphClient(context.Background(), client)
	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]any{"message_id": "draft+1", "attachment_id": "attachment+1"}
	result, err := NewHandleRemoveAttachment(graph.RetryConfig{}, time.Second)(ctx, request)
	if err != nil || result.IsError {
		t.Fatalf("result = %+v, error = %v", result, err)
	}
	want := "GET /v1.0/me/messages/draft+1|DELETE /v1.0/me/messages/draft+1/attachments/attachment+1"
	if got := strings.Join(calls, "|"); got != want {
		t.Fatalf("calls = %s, want %s", got, want)
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "Attachment removed") || !strings.Contains(text, "Attachment ID: attachment+1") {
		t.Fatalf("confirmation = %s", text)
	}
}

// TestRemoveAttachmentRejectsNonDraft verifies a non-draft message cannot
// reach the destructive attachment endpoint.
func TestRemoveAttachmentRejectsNonDraft(t *testing.T) {
	var methods []string
	client, server := newRealSDKGraphClient(t, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		methods = append(methods, request.Method)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"message+1","isDraft":false}`))
	}))
	defer server.Close()
	ctx := auth.WithGraphClient(context.Background(), client)
	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]any{"message_id": "message+1", "attachment_id": "attachment+1"}
	result, err := NewHandleRemoveAttachment(graph.RetryConfig{}, time.Second)(ctx, request)
	if err != nil || !result.IsError || strings.Join(methods, "|") != http.MethodGet {
		t.Fatalf("result = %+v, error = %v, methods = %v", result, err, methods)
	}
}

// TestRemoveSharedAttachmentUsesBoundReferences verifies shared removal keeps
// the exact owner route and returns renewed draft provenance.
func TestRemoveSharedAttachmentUsesBoundReferences(t *testing.T) {
	var calls []string
	client, server := newRealSDKGraphClient(t, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		calls = append(calls, request.Method+" "+request.URL.EscapedPath())
		w.Header().Set("Content-Type", "application/json")
		if request.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"id":"draft+1","isDraft":true}`))
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	ctx, codec := sharedDraftContext(t, client, resource.ItemKindDraft, "draft+1", func() error { return nil })
	request := sharedRemoveAttachmentRequest(t, ctx, codec, "draft+1")
	result, err := NewHandleRemoveAttachment(graph.RetryConfig{}, time.Second, &codec)(ctx, request)
	if err != nil || result.IsError {
		t.Fatalf("result = %+v, error = %v", result, err)
	}
	base := "/v1.0/users/shared%2Bops%40example.com/messages/draft%2B1"
	want := "GET " + base + "|DELETE " + base + "/attachments/attachment%2B1"
	if got := strings.Join(calls, "|"); got != want {
		t.Fatalf("calls = %s, want %s", got, want)
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "Draft Ref: v1.") || !strings.Contains(text, "Shared Target: finance-mail") {
		t.Fatalf("confirmation omitted renewed draft provenance: %s", text)
	}
}

// TestRemoveSharedAttachmentRejectsCrossParentReference verifies parent-draft
// mismatch is rejected locally before the draft preflight or deletion.
func TestRemoveSharedAttachmentRejectsCrossParentReference(t *testing.T) {
	var calls int
	client, server := newRealSDKGraphClient(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls++ }))
	defer server.Close()
	ctx, codec := sharedDraftContext(t, client, resource.ItemKindDraft, "draft+1", func() error { return nil })
	request := sharedRemoveAttachmentRequest(t, ctx, codec, "other-draft")
	result, err := NewHandleRemoveAttachment(graph.RetryConfig{}, time.Second, &codec)(ctx, request)
	if err != nil || !result.IsError || calls != 0 {
		t.Fatalf("result = %+v, error = %v, calls = %d", result, err, calls)
	}
}

// TestRemoveSharedAttachmentRevalidatesBeforeDelete verifies policy revocation
// after draft preflight prevents the destructive request.
func TestRemoveSharedAttachmentRevalidatesBeforeDelete(t *testing.T) {
	var methods []string
	client, server := newRealSDKGraphClient(t, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		methods = append(methods, request.Method)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"draft+1","isDraft":true}`))
	}))
	defer server.Close()
	ctx, codec := sharedDraftContext(t, client, resource.ItemKindDraft, "draft+1", func() error { return errors.New("draft permission was revoked") })
	request := sharedRemoveAttachmentRequest(t, ctx, codec, "draft+1")
	result, err := NewHandleRemoveAttachment(graph.RetryConfig{}, time.Second, &codec)(ctx, request)
	if err != nil || !result.IsError || strings.Join(methods, "|") != http.MethodGet {
		t.Fatalf("result = %+v, error = %v, methods = %v", result, err, methods)
	}
}

// sharedRemoveAttachmentRequest signs an attachment reference for parentID and
// returns a shared draft removal request backed by the routed draft claims.
func sharedRemoveAttachmentRequest(t *testing.T, ctx context.Context, codec resource.ReferenceCodec, parentID string) mcp.CallToolRequest {
	t.Helper()
	routed, ok := graph.RoutedTargetFromContext(ctx)
	if !ok {
		t.Fatal("shared routed target is unavailable")
	}
	reference, err := codec.Sign(resource.ReferenceClaims{
		AccountID: routed.Target.AccountID, ResourceID: routed.Target.ResourceID,
		ResourceKind: routed.Target.Kind, MailboxView: routed.Target.View, ItemKind: resource.ItemKindAttachment,
		GraphIDChain: []resource.GraphID{
			{Kind: resource.ItemKindMessage, ID: parentID},
			{Kind: resource.ItemKindAttachment, ID: "attachment+1"},
		},
	})
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}
	request := sharedDraftRequest()
	request.Params.Arguments.(map[string]any)["attachment_ref"] = reference
	return request
}
