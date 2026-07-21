package tools

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/desek/outlook-local-mcp/internal/graph"
	"github.com/desek/outlook-local-mcp/internal/resource"
	"github.com/mark3labs/mcp-go/mcp"
)

// TestSharedSmallAttachmentUsesExactOwnerRoute verifies preflight, direct
// upload, and verification retain one shared mailbox route and return target
// provenance without local path or content disclosure.
func TestSharedSmallAttachmentUsesExactOwnerRoute(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "note.txt")
	if err := os.WriteFile(path, []byte("abc"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	var paths []string
	client, server := newRealSDKGraphClient(t, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		paths = append(paths, request.Method+" "+request.URL.EscapedPath())
		w.Header().Set("Content-Type", "application/json")
		if request.Method == http.MethodGet && strings.HasSuffix(request.URL.EscapedPath(), "/draft%2B1") {
			_, _ = w.Write([]byte(`{"id":"draft+1","isDraft":true}`))
			return
		}
		_, _ = w.Write([]byte(`{"@odata.type":"#microsoft.graph.fileAttachment","id":"attachment+1","name":"note.txt","contentType":"text/plain; charset=utf-8","size":3}`))
	}))
	defer server.Close()
	ctx, codec := sharedDraftContext(t, client, resource.ItemKindDraft, "draft+1", func() error { return nil })
	request := sharedAttachmentRequest(path)
	result, err := NewHandleAddAttachment(graph.RetryConfig{}, time.Second, []string{root}, nil, &codec)(ctx, request)
	if err != nil || result.IsError {
		t.Fatalf("result = %+v, error = %v", result, err)
	}
	base := "/v1.0/users/shared%2Bops%40example.com/messages/draft%2B1"
	want := "GET " + base + "|POST " + base + "/attachments|GET " + base + "/attachments/attachment%2B1"
	if got := strings.Join(paths, "|"); got != want {
		t.Fatalf("paths = %v, want %s", paths, want)
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "Draft Ref: v1.") || !strings.Contains(text, "Shared Target: finance-mail") || strings.Contains(text, root) || strings.Contains(text, "YWJj") {
		t.Fatalf("confirmation leaked or omitted metadata: %s", text)
	}
}

// TestSharedSmallAttachmentRevalidatesBeforeUpload verifies policy revocation
// after the draft preflight stops the direct attachment mutation.
func TestSharedSmallAttachmentRevalidatesBeforeUpload(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "note.txt")
	if err := os.WriteFile(path, []byte("abc"), 0o600); err != nil {
		t.Fatal(err)
	}
	var methods []string
	client, server := newRealSDKGraphClient(t, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		methods = append(methods, request.Method)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"draft+1","isDraft":true}`))
	}))
	defer server.Close()
	ctx, codec := sharedDraftContext(t, client, resource.ItemKindDraft, "draft+1", func() error { return errors.New("draft permission was revoked") })
	result, err := NewHandleAddAttachment(graph.RetryConfig{}, time.Second, []string{root}, nil, &codec)(ctx, sharedAttachmentRequest(path))
	if err != nil || !result.IsError || strings.Join(methods, "|") != http.MethodGet {
		t.Fatalf("result = %+v, error = %v, methods = %v", result, err, methods)
	}
}

// TestSharedAttachmentRejectsSessionSizedFile verifies the evidence-limited
// shared boundary does not start a resumable upload session.
func TestSharedAttachmentRejectsSessionSizedFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "large.bin")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(attachmentChunkSize); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	_ = file.Close()
	var methods []string
	client, server := newRealSDKGraphClient(t, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		methods = append(methods, request.Method)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"draft+1","isDraft":true}`))
	}))
	defer server.Close()
	ctx, codec := sharedDraftContext(t, client, resource.ItemKindDraft, "draft+1", func() error { return nil })
	result, err := NewHandleAddAttachment(graph.RetryConfig{}, time.Second, []string{root}, nil, &codec)(ctx, sharedAttachmentRequest(path))
	if err != nil || !result.IsError || strings.Join(methods, "|") != http.MethodGet {
		t.Fatalf("result = %+v, error = %v, methods = %v", result, err, methods)
	}
}

// TestSharedAttachmentVerificationFailureReportsPartialSuccess verifies a
// completed direct POST is neither retried nor presented as wholly failed.
func TestSharedAttachmentVerificationFailureReportsPartialSuccess(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "note.txt")
	if err := os.WriteFile(path, []byte("abc"), 0o600); err != nil {
		t.Fatal(err)
	}
	var methods []string
	client, server := newRealSDKGraphClient(t, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		methods = append(methods, request.Method)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case request.Method == http.MethodGet && strings.HasSuffix(request.URL.EscapedPath(), "/draft%2B1"):
			_, _ = w.Write([]byte(`{"id":"draft+1","isDraft":true}`))
		case request.Method == http.MethodPost:
			_, _ = w.Write([]byte(`{"@odata.type":"#microsoft.graph.fileAttachment","id":"attachment+1"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"code":"ErrorItemNotFound","message":"missing"}}`))
		}
	}))
	defer server.Close()
	ctx, codec := sharedDraftContext(t, client, resource.ItemKindDraft, "draft+1", func() error { return nil })
	result, err := NewHandleAddAttachment(graph.RetryConfig{}, time.Second, []string{root}, nil, &codec)(ctx, sharedAttachmentRequest(path))
	if err != nil || result.IsError {
		t.Fatalf("result = %+v, error = %v", result, err)
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "PARTIAL SUCCESS") || !strings.Contains(text, "Attachment ID: attachment+1") || !strings.Contains(text, "Do not repeat") {
		t.Fatalf("partial confirmation = %s", text)
	}
	if strings.Join(methods, "|") != "GET|POST|GET" {
		t.Fatalf("methods = %v, want GET POST GET", methods)
	}
}

// sharedAttachmentRequest creates a shared draft attachment request backed by
// verified routed draft claims.
func sharedAttachmentRequest(path string) mcp.CallToolRequest {
	request := sharedDraftRequest()
	request.Params.Arguments.(map[string]any)["file_path"] = path
	return request
}
