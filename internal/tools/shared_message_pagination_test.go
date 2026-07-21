package tools

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/desek/outlook-local-mcp/internal/graph"
	"github.com/desek/outlook-local-mcp/internal/resource"
	"github.com/mark3labs/mcp-go/mcp"
)

// TestSharedListMessagesRejectsHostileContinuationBeforeRequest verifies the
// list handler does not let an SDK next link escape the resolved owner route.
func TestSharedListMessagesRejectsHostileContinuationBeforeRequest(t *testing.T) {
	var calls int
	client, server := newRealSDKGraphClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"value":[{"id":"message-1"}],"@odata.nextLink":"https://evil.example/v1.0/users/shared%2Bops%40example.com/messages?$skiptoken=next"}`))
	}))
	defer server.Close()
	ctx, codec, _ := sharedMailContext(t, client, nil)

	result, err := NewHandleListMessages(graph.RetryConfig{}, time.Second, "", &codec)(ctx, sharedMailRequest("summary"))
	if err != nil || !result.IsError || !strings.Contains(result.Content[0].(mcp.TextContent).Text, "continuation") {
		t.Fatalf("result = %+v, error = %v, want continuation denial", result, err)
	}
	if calls != 1 {
		t.Fatalf("Graph calls = %d, want initial collection request only", calls)
	}
}

// TestSharedSearchMessagesRejectsCrossOwnerContinuationBeforeRequest verifies
// search pagination cannot switch to another owner-view mailbox collection.
func TestSharedSearchMessagesRejectsCrossOwnerContinuationBeforeRequest(t *testing.T) {
	var calls int
	client, server := newRealSDKGraphClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"value":[{"id":"message-1"}],"@odata.nextLink":"https://graph.microsoft.com/v1.0/users/other%40example.com/messages?$skiptoken=next"}`))
	}))
	defer server.Close()
	ctx, codec, _ := sharedMailContext(t, client, nil)
	request := sharedMailRequest("summary")
	request.Params.Arguments.(map[string]any)["query"] = "subject:quarterly"

	result, err := NewHandleSearchMessages(graph.RetryConfig{}, time.Second, &codec)(ctx, request)
	if err != nil || !result.IsError || !strings.Contains(result.Content[0].(mcp.TextContent).Text, "continuation") {
		t.Fatalf("result = %+v, error = %v, want continuation denial", result, err)
	}
	if calls != 1 {
		t.Fatalf("Graph calls = %d, want initial search request only", calls)
	}
}

// TestSharedConversationRejectsRecipientViewContinuationBeforeRequest verifies
// a conversation page cannot switch from the owner collection to `/me`.
func TestSharedConversationRejectsRecipientViewContinuationBeforeRequest(t *testing.T) {
	var calls int
	client, server := newRealSDKGraphClient(t, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(request.URL.EscapedPath(), "/messages/message-1") {
			_, _ = w.Write([]byte(`{"id":"message-1","conversationId":"conversation-1"}`))
			return
		}
		_, _ = w.Write([]byte(`{"value":[{"id":"message-1","conversationId":"conversation-1"}],"@odata.nextLink":"https://graph.microsoft.com/v1.0/me/messages?$skiptoken=next"}`))
	}))
	defer server.Close()
	ctx, codec, target := sharedMailContext(t, client, nil)
	claims := resource.ReferenceClaims{
		AccountID: target.AccountID, ResourceID: target.ResourceID, ResourceKind: target.Kind,
		MailboxView: target.View, ItemKind: resource.ItemKindMessage,
		GraphIDChain: []resource.GraphID{{Kind: resource.ItemKindMessage, ID: "message-1"}},
	}
	ctx = sharedMailClaimsContext(t, ctx, target, claims)
	request := sharedMailRequest("summary")
	request.Params.Arguments.(map[string]any)["message_ref"] = "guard-verified"

	result, err := NewHandleGetConversation(graph.RetryConfig{}, time.Second, "", &codec)(ctx, request)
	if err != nil || !result.IsError || !strings.Contains(result.Content[0].(mcp.TextContent).Text, "continuation") {
		t.Fatalf("result = %+v, error = %v, want continuation denial", result, err)
	}
	if calls != 2 {
		t.Fatalf("Graph calls = %d, want message resolution and initial conversation collection only", calls)
	}
}

// TestSharedListMessagesRevalidatesEveryContinuation verifies a valid first
// continuation does not authorize a hostile next link returned by page two.
func TestSharedListMessagesRevalidatesEveryContinuation(t *testing.T) {
	var calls int
	client, server := newRealSDKGraphClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		if calls == 1 {
			_, _ = w.Write([]byte(`{"value":[{"id":"message-1"}],"@odata.nextLink":"https://graph.microsoft.com/v1.0/users/shared%2Bops%40example.com/messages?$skiptoken=page-2"}`))
			return
		}
		_, _ = w.Write([]byte(`{"value":[{"id":"message-2"}],"@odata.nextLink":"https://evil.example/v1.0/users/shared%2Bops%40example.com/messages?$skiptoken=page-3"}`))
	}))
	defer server.Close()
	ctx, codec, _ := sharedMailContext(t, client, nil)

	result, err := NewHandleListMessages(graph.RetryConfig{}, time.Second, "", &codec)(ctx, sharedMailRequest("summary"))
	if err != nil || !result.IsError {
		t.Fatalf("result = %+v, error = %v, want second-continuation denial", result, err)
	}
	if calls != 2 {
		t.Fatalf("Graph calls = %d, want initial request plus one accepted continuation", calls)
	}
}
