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

// TestRestoreRequiresDeletedItemsAndOrdinaryDestination verifies current
// source classification, exact destination resolution, and the new reference.
func TestRestoreRequiresDeletedItemsAndOrdinaryDestination(t *testing.T) {
	var paths []string
	client, server := newRealSDKGraphClient(t, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		paths = append(paths, request.Method+" "+request.URL.EscapedPath())
		w.Header().Set("Content-Type", "application/json")
		switch len(paths) {
		case 1:
			_, _ = w.Write([]byte(`{"id":"source+1","parentFolderId":"deleted-id"}`))
		case 2:
			_, _ = w.Write([]byte(`{"id":"deleted-id"}`))
		case 3:
			_, _ = w.Write([]byte(`{"id":"ordinary+1"}`))
		default:
			_, _ = w.Write([]byte(`{"id":"restored+1"}`))
		}
	}))
	defer server.Close()
	ctx, codec, target := moveMessageContext(t, client, true)
	destination := signFolderClass(t, codec, target, "ordinary+1", resource.MailFolderClassOrdinary)
	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]any{"destination_folder_ref": destination}
	result, err := NewHandleRestoreMessage(graph.RetryConfig{}, time.Second, &codec)(ctx, request)
	if err != nil || result.IsError {
		t.Fatalf("result = %+v, error = %v", result, err)
	}
	if len(paths) != 4 || !strings.HasSuffix(paths[3], "/messages/source%2B1/move") || !strings.Contains(result.Content[0].(mcp.TextContent).Text, "Resource Ref: v1.") {
		t.Fatalf("paths = %v, result = %+v", paths, result)
	}
}

// TestRestoreRejectsNonDeletedSourceBeforeMove verifies Inbox or other source
// messages cannot use restore as a general move capability.
func TestRestoreRejectsNonDeletedSourceBeforeMove(t *testing.T) {
	var calls int
	client, server := newRealSDKGraphClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		if calls == 1 {
			_, _ = w.Write([]byte(`{"id":"source+1","parentFolderId":"inbox-id"}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":"deleted-id"}`))
	}))
	defer server.Close()
	ctx, codec, target := moveMessageContext(t, client, false)
	destination := signFolderClass(t, codec, target, "ordinary-1", resource.MailFolderClassOrdinary)
	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]any{"destination_folder_ref": destination}
	result, err := NewHandleRestoreMessage(graph.RetryConfig{}, time.Second, &codec)(ctx, request)
	if err != nil || !result.IsError || calls != 2 {
		t.Fatalf("result = %+v, calls = %d, error = %v", result, calls, err)
	}
}
