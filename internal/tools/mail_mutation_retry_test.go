package tools

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/desek/outlook-local-mcp/internal/auth"
	"github.com/desek/outlook-local-mcp/internal/graph"
	"github.com/mark3labs/mcp-go/mcp"
)

// TestIrreversibleMailMutationsDisableSDKRetries verifies the Kiota retry
// middleware cannot repeat ambiguous, non-idempotent mail mutations.
func TestIrreversibleMailMutationsDisableSDKRetries(t *testing.T) {
	statuses := []int{http.StatusTooManyRequests, http.StatusServiceUnavailable, http.StatusGatewayTimeout}
	mutations := []struct {
		name string
		run  func(*testing.T, int) int
	}{
		{name: "shared_send", run: sharedSendTransientCalls},
		{name: "permanent_delete", run: permanentDeleteTransientCalls},
		{name: "direct_attachment", run: directAttachmentTransientCalls},
		{name: "remove_attachment", run: removeAttachmentTransientCalls},
		{name: "create_draft", run: createDraftTransientCalls},
		{name: "move", run: moveTransientCalls},
	}
	for _, mutation := range mutations {
		for _, status := range statuses {
			t.Run(mutation.name+"_"+http.StatusText(status), func(t *testing.T) {
				if calls := mutation.run(t, status); calls != 1 {
					t.Fatalf("mutation calls = %d, want exactly one after HTTP %d", calls, status)
				}
			})
		}
	}
}

// removeAttachmentTransientCalls returns the attachment DELETE count after a
// successful own-draft preflight and a transient mutation response.
func removeAttachmentTransientCalls(t *testing.T, status int) int {
	t.Helper()
	var calls int
	client, server := newRealSDKGraphClient(t, transientMutationHandler(&calls, status, func(w http.ResponseWriter, request *http.Request) bool {
		if request.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"draft+1","isDraft":true}`))
			return true
		}
		return false
	}))
	defer server.Close()
	ctx := auth.WithGraphClient(context.Background(), client)
	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]any{"message_id": "draft+1", "attachment_id": "attachment+1"}
	_, _ = NewHandleRemoveAttachment(graph.RetryConfig{}, time.Second)(ctx, request)
	return calls
}

// sharedSendTransientCalls returns the number of owner-route send POSTs made
// after Graph responds with status.
func sharedSendTransientCalls(t *testing.T, status int) int {
	t.Helper()
	var calls int
	client, server := newRealSDKGraphClient(t, transientMutationHandler(&calls, status, nil))
	defer server.Close()
	ctx, _, _ := sharedDraftSendContext(t, client)
	target, _ := mailTargetFromContext(ctx)
	_ = sendSharedDraftOnce(ctx, target, "draft+1", time.Second)
	return calls
}

// permanentDeleteTransientCalls returns the destructive action POST count
// after Graph responds with status.
func permanentDeleteTransientCalls(t *testing.T, status int) int {
	t.Helper()
	var calls int
	client, server := newRealSDKGraphClient(t, transientMutationHandler(&calls, status, nil))
	defer server.Close()
	ctx, _, _ := moveMessageContext(t, client, true)
	target, _ := mailTargetFromContext(ctx)
	_ = permanentlyDeleteMessageOnce(ctx, target, "source+1", time.Second)
	return calls
}

// directAttachmentTransientCalls returns the direct attachment POST count
// after a successful draft check and a transient mutation response.
func directAttachmentTransientCalls(t *testing.T, status int) int {
	t.Helper()
	var calls int
	client, server := newRealSDKGraphClient(t, transientMutationHandler(&calls, status, func(w http.ResponseWriter, request *http.Request) bool {
		if request.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"draft+1","isDraft":true}`))
			return true
		}
		return false
	}))
	defer server.Close()
	ctx, codec, _ := sharedDraftSendContext(t, client)
	dir := t.TempDir()
	path := filepath.Join(dir, "note.txt")
	if err := os.WriteFile(path, []byte("note"), 0o600); err != nil {
		t.Fatal(err)
	}
	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]any{"draft_ref": "v1.signed", "file_path": path}
	_, _ = NewHandleAddAttachment(graph.RetryConfig{}, time.Second, []string{dir}, nil, &codec)(ctx, request)
	return calls
}

// createDraftTransientCalls returns the new-message POST count after Graph
// responds to draft creation with status.
func createDraftTransientCalls(t *testing.T, status int) int {
	t.Helper()
	var calls int
	client, server := newRealSDKGraphClient(t, transientMutationHandler(&calls, status, nil))
	defer server.Close()
	ctx, _, _ := moveMessageContext(t, client, false)
	_, _ = NewHandleCreateDraft(graph.RetryConfig{}, time.Second, "", nil)(ctx, mcp.CallToolRequest{})
	return calls
}

// moveTransientCalls returns the move action POST count after Graph responds
// with status.
func moveTransientCalls(t *testing.T, status int) int {
	t.Helper()
	var calls int
	client, server := newRealSDKGraphClient(t, transientMutationHandler(&calls, status, nil))
	defer server.Close()
	ctx, codec, _ := moveMessageContext(t, client, false)
	target, _ := mailTargetFromContext(ctx)
	_, _ = moveMessageToSemanticDestination(ctx, target, "source+1", "archive", "archive", time.Second, &codec)
	return calls
}

// transientMutationHandler optionally serves prerequisite reads, then records
// mutation traffic and returns the configured retryable Graph response.
func transientMutationHandler(calls *int, status int, before func(http.ResponseWriter, *http.Request) bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if before != nil && before(w, request) {
			return
		}
		*calls++
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"error":{"code":"Transient","message":"retry"}}`))
	})
}
