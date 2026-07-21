package tools

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

// TestPermanentDeleteRequiresFreshUnchangedConfirmation verifies decline and
// stale review stop locally while one accepted unchanged review dispatches.
func TestPermanentDeleteRequiresFreshUnchangedConfirmation(t *testing.T) {
	tests := []struct {
		name       string
		response   mcp.ElicitationResponseAction
		changeKeys []string
		wantDelete int
		wantError  bool
	}{
		{name: "declined", response: mcp.ElicitationResponseActionDecline, changeKeys: []string{"v1"}},
		{name: "stale", response: mcp.ElicitationResponseActionAccept, changeKeys: []string{"v1", "v2"}, wantError: true},
		{name: "accepted", response: mcp.ElicitationResponseActionAccept, changeKeys: []string{"v1", "v1"}, wantDelete: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client, server := newRealSDKGraphClient(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
			defer server.Close()
			ctx, _, _ := moveMessageContext(t, client, true)
			loadIndex, deletes := 0, 0
			state := &permanentDeleteState{
				load: func(context.Context, mailReadTarget, string) (permanentDeleteSummary, error) {
					key := test.changeKeys[loadIndex]
					loadIndex++
					return permanentDeleteSummary{ChangeKey: key, Subject: "Quarterly review"}, nil
				},
				elicit: func(_ context.Context, request mcp.ElicitationRequest) (*mcp.ElicitationResult, error) {
					if !strings.Contains(request.Params.Message, "PERMANENTLY DELETE") || !strings.Contains(request.Params.Message, "sh***@example.com") {
						t.Fatalf("elicitation = %s", request.Params.Message)
					}
					return &mcp.ElicitationResult{ElicitationResponse: mcp.ElicitationResponse{Action: test.response, Content: map[string]any{"confirmed": true}}}, nil
				},
				delete: func(context.Context, mailReadTarget, string) error { deletes++; return nil },
			}
			request := mcp.CallToolRequest{}
			request.Params.Arguments = map[string]any{"message_ref": "v1.signed"}
			result, err := handlePermanentDeleteMessage(state)(ctx, request)
			if err != nil || result.IsError != test.wantError || deletes != test.wantDelete {
				t.Fatalf("result = %+v, deletes = %d, error = %v", result, deletes, err)
			}
		})
	}
}

// TestPermanentDeleteUsesDistinctExactTargetRoute verifies accepted review
// dispatches the generated permanentDelete action exactly once.
func TestPermanentDeleteUsesDistinctExactTargetRoute(t *testing.T) {
	var paths []string
	client, server := newRealSDKGraphClient(t, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		paths = append(paths, request.Method+" "+request.URL.EscapedPath())
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	ctx, _, _ := moveMessageContext(t, client, true)
	state := &permanentDeleteState{
		load: func(context.Context, mailReadTarget, string) (permanentDeleteSummary, error) {
			return permanentDeleteSummary{ChangeKey: "stable", Subject: "Review"}, nil
		},
		elicit: func(context.Context, mcp.ElicitationRequest) (*mcp.ElicitationResult, error) {
			return &mcp.ElicitationResult{ElicitationResponse: mcp.ElicitationResponse{
				Action: mcp.ElicitationResponseActionAccept, Content: map[string]any{"confirmed": true},
			}}, nil
		},
		delete: func(ctx context.Context, target mailReadTarget, messageID string) error {
			return permanentlyDeleteMessageOnce(ctx, target, messageID, time.Second)
		},
	}
	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]any{"message_ref": "v1.signed"}
	result, err := handlePermanentDeleteMessage(state)(ctx, request)
	if err != nil || result.IsError || len(paths) != 1 || paths[0] != "POST /v1.0/users/shared%2Bops%40example.com/messages/source%2B1/permanentDelete" {
		t.Fatalf("result = %+v, paths = %v, error = %v", result, paths, err)
	}
}
