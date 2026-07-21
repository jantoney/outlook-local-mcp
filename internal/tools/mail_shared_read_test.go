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

// TestSharedMailReadsUseExactOwnerRoute verifies folder, scoped message, and
// message-detail reads use one configured owner view and signed references.
func TestSharedMailReadsUseExactOwnerRoute(t *testing.T) {
	var paths []string
	client, server := newRealSDKGraphClient(t, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		paths = append(paths, request.URL.EscapedPath())
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(request.URL.EscapedPath(), "/mailFolders"):
			_, _ = w.Write([]byte(`{"value":[{"id":"folder+1","displayName":"Inbox","unreadItemCount":1,"totalItemCount":2}]}`))
		case strings.Contains(request.URL.EscapedPath(), "/messages/message"):
			_, _ = w.Write([]byte(`{"id":"message+1","subject":"Shared detail","bodyPreview":"Preview"}`))
		default:
			_, _ = w.Write([]byte(`{"value":[{"id":"message+1","subject":"Shared list","bodyPreview":"Preview"}]}`))
		}
	}))
	defer server.Close()
	ctx, codec, target := sharedMailContext(t, client, nil)

	folderResult, err := NewHandleListMailFolders(graph.RetryConfig{}, time.Second, &codec)(ctx, sharedMailRequest("summary"))
	if err != nil || folderResult.IsError {
		t.Fatalf("folders result = %+v, error = %v", folderResult, err)
	}
	folderRef := firstSharedReference(t, folderResult)
	folderClaims, err := codec.Verify(folderRef)
	if err != nil || folderClaims.ItemKind != resource.ItemKindMailFolder {
		t.Fatalf("folder claims = %+v, error = %v", folderClaims, err)
	}
	folderCtx := sharedMailClaimsContext(t, ctx, target, folderClaims)
	listRequest := sharedMailRequest("summary")
	listRequest.Params.Arguments.(map[string]any)["folder_ref"] = folderRef
	messageResult, err := NewHandleListMessages(graph.RetryConfig{}, time.Second, "", &codec)(folderCtx, listRequest)
	if err != nil || messageResult.IsError {
		t.Fatalf("messages result = %+v, error = %v", messageResult, err)
	}
	messageRef := firstSharedReference(t, messageResult)
	messageClaims, err := codec.Verify(messageRef)
	if err != nil || messageClaims.ItemKind != resource.ItemKindMessage {
		t.Fatalf("message claims = %+v, error = %v", messageClaims, err)
	}
	messageCtx := sharedMailClaimsContext(t, ctx, target, messageClaims)
	getRequest := sharedMailRequest("summary")
	getRequest.Params.Arguments.(map[string]any)["resource_ref"] = messageRef
	getResult, err := NewHandleGetMessage(graph.RetryConfig{}, time.Second, "", &codec)(messageCtx, getRequest)
	if err != nil || getResult.IsError {
		t.Fatalf("get result = %+v, error = %v", getResult, err)
	}

	want := []string{
		"/v1.0/users/shared%2Bops%40example.com/mailFolders",
		"/v1.0/users/shared%2Bops%40example.com/mailFolders/folder%2B1/messages",
		"/v1.0/users/shared%2Bops%40example.com/messages/message%2B1",
	}
	if strings.Join(paths, "|") != strings.Join(want, "|") {
		t.Fatalf("paths = %v, want %v", paths, want)
	}
}

// TestSharedMailRawUsesProvenanceSidecar verifies raw Graph-derived message
// data is not injected with local reference fields.
func TestSharedMailRawUsesProvenanceSidecar(t *testing.T) {
	client, server := newRealSDKGraphClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"value":[{"id":"message-1","subject":"Raw"}]}`))
	}))
	defer server.Close()
	ctx, codec, _ := sharedMailContext(t, client, nil)
	result, err := NewHandleListMessages(graph.RetryConfig{}, time.Second, "", &codec)(ctx, sharedMailRequest("raw"))
	if err != nil || result.IsError {
		t.Fatalf("result = %+v, error = %v", result, err)
	}
	var body map[string]any
	text := result.Content[0].(mcp.TextContent).Text
	if err := json.Unmarshal([]byte(text), &body); err != nil {
		t.Fatalf("raw output = %s, error = %v", text, err)
	}
	data := body["data"].([]any)[0].(map[string]any)
	if _, injected := data["resource_ref"]; injected || body["provenance"] == nil {
		t.Fatalf("raw envelope = %+v", body)
	}
}

// TestSharedMailTextExposesReferences verifies human-readable folder and
// message collections carry copyable target-bound references.
func TestSharedMailTextExposesReferences(t *testing.T) {
	client, server := newRealSDKGraphClient(t, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(request.URL.EscapedPath(), "/mailFolders") {
			_, _ = w.Write([]byte(`{"value":[{"id":"folder-1","displayName":"Inbox"}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"value":[{"id":"message-1","subject":"Shared"}]}`))
	}))
	defer server.Close()
	ctx, codec, _ := sharedMailContext(t, client, nil)
	for name, call := range map[string]func() (*mcp.CallToolResult, error){
		"folders": func() (*mcp.CallToolResult, error) {
			return NewHandleListMailFolders(graph.RetryConfig{}, time.Second, &codec)(ctx, sharedMailRequest("text"))
		},
		"messages": func() (*mcp.CallToolResult, error) {
			return NewHandleListMessages(graph.RetryConfig{}, time.Second, "", &codec)(ctx, sharedMailRequest("text"))
		},
	} {
		t.Run(name, func(t *testing.T) {
			result, err := call()
			if err != nil || result.IsError {
				t.Fatalf("result = %+v, error = %v", result, err)
			}
			if text := result.Content[0].(mcp.TextContent).Text; !strings.Contains(text, "Resource Ref: v1.") {
				t.Fatalf("text lacks resource reference: %s", text)
			}
		})
	}
}

// sharedMailContext creates one authorized owner-view shared mailbox target.
func sharedMailContext(t *testing.T, client *msgraphsdk.GraphServiceClient, claims *resource.ReferenceClaims) (context.Context, resource.ReferenceCodec, resource.Target) {
	t.Helper()
	key, err := resource.LoadOrCreateSigningKey(filepath.Join(t.TempDir(), "reference.key"))
	if err != nil {
		t.Fatalf("LoadOrCreateSigningKey() error = %v", err)
	}
	codec := resource.NewReferenceCodec(key)
	alias, err := resource.NewMailAlias(resource.ResourceID("22222222-2222-4222-8222-222222222222"), "finance-mail", "shared+ops@example.com")
	if err != nil {
		t.Fatalf("NewMailAlias() error = %v", err)
	}
	alias.Policy.Read = true
	target, err := resource.TargetFromMailAlias(resource.AccountID("11111111-1111-4111-8111-111111111111"), alias)
	if err != nil {
		t.Fatalf("TargetFromMailAlias() error = %v", err)
	}
	root, err := graph.SelectUserRoot(client, target)
	if err != nil {
		t.Fatalf("SelectUserRoot() error = %v", err)
	}
	ctx := auth.WithGraphClient(context.Background(), client)
	ctx = graph.WithRoutedTarget(ctx, graph.RoutedTarget{Target: target, Root: root, Claims: claims})
	return ctx, codec, target
}

// sharedMailClaimsContext replaces only the verified claims on a routed target.
func sharedMailClaimsContext(t *testing.T, ctx context.Context, target resource.Target, claims resource.ReferenceClaims) context.Context {
	t.Helper()
	routed, ok := graph.RoutedTargetFromContext(ctx)
	if !ok || routed.Target != target {
		t.Fatal("shared mail routed target missing")
	}
	routed.Claims = &claims
	return graph.WithRoutedTarget(ctx, routed)
}

// sharedMailRequest creates one shared read request with an output tier.
func sharedMailRequest(output string) mcp.CallToolRequest {
	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]any{"shared_resource": "finance-mail", "output": output}
	return request
}

// firstSharedReference extracts the first summary reference.
func firstSharedReference(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	var items []map[string]any
	text := result.Content[0].(mcp.TextContent).Text
	if err := json.Unmarshal([]byte(text), &items); err != nil || len(items) == 0 {
		t.Fatalf("summary output = %s, error = %v", text, err)
	}
	reference, _ := items[0]["resource_ref"].(string)
	if reference == "" {
		t.Fatalf("summary lacks resource_ref: %s", text)
	}
	return reference
}
