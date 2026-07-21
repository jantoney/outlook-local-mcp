package tools

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/desek/outlook-local-mcp/internal/graph"
	"github.com/desek/outlook-local-mcp/internal/resource"
	"github.com/mark3labs/mcp-go/mcp"
)

// TestSharedMailSearchConversationAndAttachmentsUseOwnerRoute verifies the
// remaining shared read surface stays in one owner view and chains provenance.
func TestSharedMailSearchConversationAndAttachmentsUseOwnerRoute(t *testing.T) {
	var paths []string
	client, server := newRealSDKGraphClient(t, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		paths = append(paths, request.URL.EscapedPath())
		w.Header().Set("Content-Type", "application/json")
		path := request.URL.EscapedPath()
		switch {
		case strings.Contains(path, "/attachments/attachment"):
			_, _ = w.Write([]byte(`{"@odata.type":"#microsoft.graph.fileAttachment","id":"attachment+1","name":"note.txt","contentType":"text/plain","size":3,"contentBytes":"YWJj"}`))
		case strings.HasSuffix(path, "/attachments"):
			_, _ = w.Write([]byte(`{"value":[{"@odata.type":"#microsoft.graph.fileAttachment","id":"attachment+1","name":"note.txt","contentType":"text/plain","size":3}]}`))
		case strings.Contains(path, "/messages/message"):
			_, _ = w.Write([]byte(`{"id":"message+1","conversationId":"conversation+1"}`))
		default:
			_, _ = w.Write([]byte(`{"value":[{"id":"message+1","conversationId":"conversation+1","subject":"Shared thread","bodyPreview":"Preview"}]}`))
		}
	}))
	defer server.Close()
	ctx, codec, target := sharedMailContext(t, client, nil)

	searchRequest := sharedMailRequest("summary")
	searchRequest.Params.Arguments.(map[string]any)["query"] = "subject:Shared"
	searchResult, err := NewHandleSearchMessages(graph.RetryConfig{}, time.Second, &codec)(ctx, searchRequest)
	if err != nil || searchResult.IsError {
		t.Fatalf("search result = %+v, error = %v", searchResult, err)
	}
	messageRef := firstSharedReference(t, searchResult)
	messageClaims, err := codec.Verify(messageRef)
	if err != nil {
		t.Fatalf("Verify(message) error = %v", err)
	}
	messageCtx := sharedMailClaimsContext(t, ctx, target, messageClaims)

	conversationRequest := sharedMailRequest("summary")
	conversationRequest.Params.Arguments.(map[string]any)["message_ref"] = messageRef
	conversationResult, err := NewHandleGetConversation(graph.RetryConfig{}, time.Second, "", &codec)(messageCtx, conversationRequest)
	if err != nil || conversationResult.IsError || !strings.Contains(conversationResult.Content[0].(mcp.TextContent).Text, "resource_ref") {
		t.Fatalf("conversation result = %+v, error = %v", conversationResult, err)
	}

	attachmentRequest := sharedMailRequest("summary")
	attachmentRequest.Params.Arguments.(map[string]any)["message_ref"] = messageRef
	attachmentResult, err := NewHandleListAttachments(graph.RetryConfig{}, time.Second, &codec)(messageCtx, attachmentRequest)
	if err != nil || attachmentResult.IsError {
		t.Fatalf("attachments result = %+v, error = %v", attachmentResult, err)
	}
	attachmentRef := firstAttachmentReference(t, attachmentResult)

	downloadRequest := sharedMailRequest("summary")
	downloadRequest.Params.Arguments.(map[string]any)["message_ref"] = messageRef
	downloadRequest.Params.Arguments.(map[string]any)["attachment_ref"] = attachmentRef
	downloadResult, err := NewHandleGetAttachment(graph.RetryConfig{}, time.Second, 1024, &codec)(messageCtx, downloadRequest)
	if err != nil || downloadResult.IsError || !strings.Contains(downloadResult.Content[0].(mcp.TextContent).Text, "attachment_ref") {
		t.Fatalf("download result = %+v, error = %v", downloadResult, err)
	}

	want := []string{
		"/v1.0/users/shared%2Bops%40example.com/messages",
		"/v1.0/users/shared%2Bops%40example.com/messages/message%2B1",
		"/v1.0/users/shared%2Bops%40example.com/messages",
		"/v1.0/users/shared%2Bops%40example.com/messages/message%2B1/attachments",
		"/v1.0/users/shared%2Bops%40example.com/messages/message%2B1/attachments/attachment%2B1",
	}
	if strings.Join(paths, "|") != strings.Join(want, "|") {
		t.Fatalf("paths = %v, want %v", paths, want)
	}
}

// TestSharedAttachmentCrossParentFailsBeforeGraph verifies an attachment ref
// cannot be replayed beneath another verified message in the same mailbox.
func TestSharedAttachmentCrossParentFailsBeforeGraph(t *testing.T) {
	var calls int
	client, server := newRealSDKGraphClient(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls++ }))
	defer server.Close()
	ctx, codec, target := sharedMailContext(t, client, nil)
	messageClaims := resource.ReferenceClaims{
		AccountID: target.AccountID, ResourceID: target.ResourceID, ResourceKind: target.Kind,
		MailboxView: target.View, ItemKind: resource.ItemKindMessage,
		GraphIDChain: []resource.GraphID{{Kind: resource.ItemKindMessage, ID: "message-1"}},
	}
	ctx = sharedMailClaimsContext(t, ctx, target, messageClaims)
	attachmentRef, err := codec.Sign(resource.ReferenceClaims{
		AccountID: target.AccountID, ResourceID: target.ResourceID, ResourceKind: target.Kind,
		MailboxView: target.View, ItemKind: resource.ItemKindAttachment,
		GraphIDChain: []resource.GraphID{
			{Kind: resource.ItemKindMessage, ID: "other-message"},
			{Kind: resource.ItemKindAttachment, ID: "attachment-1"},
		},
	})
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}
	request := sharedMailRequest("summary")
	request.Params.Arguments.(map[string]any)["attachment_ref"] = attachmentRef
	result, err := NewHandleGetAttachment(graph.RetryConfig{}, time.Second, 1024, &codec)(ctx, request)
	if err != nil || !result.IsError || calls != 0 {
		t.Fatalf("result = %+v, error = %v, Graph calls = %d", result, err, calls)
	}
}

// TestSharedMailSearchAndConversationPagingStayOnOwner verifies generated-SDK
// next links never change the resolved shared mailbox route.
func TestSharedMailSearchAndConversationPagingStayOnOwner(t *testing.T) {
	var paths []string
	client, server := newRealSDKGraphClient(t, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		paths = append(paths, request.URL.EscapedPath())
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(request.URL.EscapedPath(), "/messages/message") {
			_, _ = w.Write([]byte(`{"id":"message-1","conversationId":"conversation-1"}`))
			return
		}
		if request.URL.Query().Get("$skiptoken") != "" {
			_, _ = w.Write([]byte(`{"value":[{"id":"message-2","conversationId":"conversation-1","subject":"Second"}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"value":[{"id":"message-1","conversationId":"conversation-1","subject":"First"}],"@odata.nextLink":"https://graph.microsoft.com/v1.0/users/shared%2Bops%40example.com/messages?$skiptoken=next"}`))
	}))
	defer server.Close()
	ctx, codec, target := sharedMailContext(t, client, nil)
	searchRequest := sharedMailRequest("summary")
	searchRequest.Params.Arguments.(map[string]any)["query"] = "subject:thread"
	searchResult, err := NewHandleSearchMessages(graph.RetryConfig{}, time.Second, &codec)(ctx, searchRequest)
	if err != nil || searchResult.IsError {
		t.Fatalf("search result = %+v, error = %v", searchResult, err)
	}
	claims := resource.ReferenceClaims{
		AccountID: target.AccountID, ResourceID: target.ResourceID, ResourceKind: target.Kind,
		MailboxView: target.View, ItemKind: resource.ItemKindMessage,
		GraphIDChain: []resource.GraphID{{Kind: resource.ItemKindMessage, ID: "message-1"}},
	}
	messageCtx := sharedMailClaimsContext(t, ctx, target, claims)
	conversationRequest := sharedMailRequest("summary")
	conversationRequest.Params.Arguments.(map[string]any)["message_ref"] = "guard-verified"
	conversationResult, err := NewHandleGetConversation(graph.RetryConfig{}, time.Second, "", &codec)(messageCtx, conversationRequest)
	if err != nil || conversationResult.IsError {
		t.Fatalf("conversation result = %+v, error = %v", conversationResult, err)
	}
	ownerCollection := "/v1.0/users/shared%2Bops%40example.com/messages"
	for _, path := range paths {
		if strings.Contains(path, "/messages/") {
			continue
		}
		if path != ownerCollection {
			t.Fatalf("paged path = %q, want %q; all paths = %v", path, ownerCollection, paths)
		}
	}
	if len(paths) != 5 {
		t.Fatalf("paths = %v, want search two pages plus conversation resolution and two pages", paths)
	}
}

// firstAttachmentReference extracts the first summary attachment reference.
func firstAttachmentReference(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	var items []map[string]any
	text := result.Content[0].(mcp.TextContent).Text
	if err := json.Unmarshal([]byte(text), &items); err != nil || len(items) == 0 {
		t.Fatalf("attachment summary = %s, error = %v", text, err)
	}
	reference, _ := items[0]["attachment_ref"].(string)
	if reference == "" {
		t.Fatalf("attachment summary lacks attachment_ref: %s", text)
	}
	return reference
}
