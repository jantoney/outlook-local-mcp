package tools

import (
	"context"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/desek/outlook-local-mcp/internal/graph"
	"github.com/desek/outlook-local-mcp/internal/resource"
	"github.com/mark3labs/mcp-go/mcp"
	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"
)

// TestSharedSendRequiresExactConfirmationAndStableSnapshot verifies declined
// and stale review stop locally while an unchanged acceptance sends once.
func TestSharedSendRequiresExactConfirmationAndStableSnapshot(t *testing.T) {
	base := sharedSendSnapshot{
		AccountID: "11111111-1111-4111-8111-111111111111", ResourceID: "22222222-2222-4222-8222-222222222222",
		Alias: "finance-mail", Owner: "shared+ops@example.com", View: "owner", From: "shared+ops@example.com",
		ChangeKey: "v1", Subject: "Review", To: []string{"a@example.com"},
		Attachments: []sharedSendAttachment{{ID: "a1", Name: "report.pdf", ContentType: "application/pdf", Size: 12}},
	}
	tests := []struct {
		name       string
		action     mcp.ElicitationResponseAction
		changeKeys []string
		wantSend   int
		wantError  bool
	}{
		{name: "declined", action: mcp.ElicitationResponseActionDecline, changeKeys: []string{"v1"}},
		{name: "stale", action: mcp.ElicitationResponseActionAccept, changeKeys: []string{"v1", "v2"}, wantError: true},
		{name: "accepted", action: mcp.ElicitationResponseActionAccept, changeKeys: []string{"v1", "v1"}, wantSend: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client, server := newRealSDKGraphClient(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
			defer server.Close()
			ctx, codec, _ := sharedDraftSendContext(t, client)
			loadIndex, sends := 0, 0
			state := &sharedSendState{
				load: func(context.Context, mailReadTarget, string) (sharedSendSnapshot, error) {
					snapshot := base
					snapshot.ChangeKey = test.changeKeys[loadIndex]
					loadIndex++
					return snapshot, nil
				},
				elicit: func(_ context.Context, request mcp.ElicitationRequest) (*mcp.ElicitationResult, error) {
					if !strings.Contains(request.Params.Message, "body is not included") || !strings.Contains(request.Params.Message, "report.pdf") || strings.Contains(request.Params.Message, "a@example.com") {
						t.Fatalf("review prompt = %s", request.Params.Message)
					}
					return &mcp.ElicitationResult{ElicitationResponse: mcp.ElicitationResponse{
						Action: test.action, Content: map[string]any{"confirmed": true},
					}}, nil
				},
				send: func(context.Context, mailReadTarget, string) error { sends++; return nil },
			}
			request := mcp.CallToolRequest{}
			request.Params.Arguments = map[string]any{"draft_ref": "v1.signed"}
			result, err := handleSharedSendDraft(state, &codec)(ctx, request)
			if err != nil || result.IsError != test.wantError || sends != test.wantSend {
				t.Fatalf("result = %+v, sends = %d, error = %v", result, sends, err)
			}
		})
	}
}

// TestSharedSendUsesOneOwnerRoutePost verifies the accepted dispatch does not
// fall back to /me or retry the non-idempotent send action.
func TestSharedSendUsesOneOwnerRoutePost(t *testing.T) {
	var paths []string
	client, server := newRealSDKGraphClient(t, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		paths = append(paths, request.Method+" "+request.URL.EscapedPath())
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()
	ctx, codec, _ := sharedDraftSendContext(t, client)
	snapshot := sharedSendSnapshot{AccountID: "a", ResourceID: "r", Alias: "finance-mail", Owner: "shared+ops@example.com", View: "owner", From: "shared+ops@example.com", ChangeKey: "v1", To: []string{"a@example.com"}}
	state := &sharedSendState{
		load: func(context.Context, mailReadTarget, string) (sharedSendSnapshot, error) { return snapshot, nil },
		elicit: func(context.Context, mcp.ElicitationRequest) (*mcp.ElicitationResult, error) {
			return &mcp.ElicitationResult{ElicitationResponse: mcp.ElicitationResponse{Action: mcp.ElicitationResponseActionAccept, Content: map[string]any{"confirmed": true}}}, nil
		},
		send: func(ctx context.Context, target mailReadTarget, draftID string) error {
			return sendSharedDraftOnce(ctx, target, draftID, time.Second)
		},
	}
	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]any{"draft_ref": "v1.signed"}
	result, err := handleSharedSendDraft(state, &codec)(ctx, request)
	if err != nil || result.IsError || !reflect.DeepEqual(paths, []string{"POST /v1.0/users/shared%2Bops%40example.com/messages/draft%2B1/send"}) {
		t.Fatalf("result = %+v, paths = %v, error = %v", result, paths, err)
	}
}

// TestSharedAttachmentNextLinkCannotChangeOwnerRoute verifies pagination is
// bound to the configured owner and draft collection.
func TestSharedAttachmentNextLinkCannotChangeOwnerRoute(t *testing.T) {
	valid := "https://graph.microsoft.com/v1.0/users/shared+ops@example.com/messages/draft+1/attachments?$skip=1"
	if err := validateSharedAttachmentNextLink(valid, "shared+ops@example.com", "draft+1"); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []string{
		"https://graph.microsoft.com/v1.0/me/messages/draft+1/attachments?$skip=1",
		"https://graph.microsoft.com/v1.0/users/other@example.com/messages/draft+1/attachments?$skip=1",
		"http://graph.microsoft.com/v1.0/users/shared+ops@example.com/messages/draft+1/attachments?$skip=1",
		"https://attacker.example/v1.0/users/shared+ops@example.com/messages/draft+1/attachments?$skip=1",
	} {
		if err := validateSharedAttachmentNextLink(invalid, "shared+ops@example.com", "draft+1"); err == nil {
			t.Fatalf("accepted invalid next link %s", invalid)
		}
	}
}

// sharedDraftSendContext converts the shared move fixture into a verified
// shared-draft route for direct handler tests.
func sharedDraftSendContext(t *testing.T, client *msgraphsdk.GraphServiceClient) (context.Context, resource.ReferenceCodec, resource.Target) {
	t.Helper()
	ctx, codec, target := moveMessageContext(t, client, true)
	routed, ok := graph.RoutedTargetFromContext(ctx)
	if !ok {
		t.Fatal("missing routed target")
	}
	claims := resource.ReferenceClaims{
		AccountID: target.AccountID, ResourceID: target.ResourceID, ResourceKind: target.Kind,
		MailboxView: target.View, ItemKind: resource.ItemKindDraft,
		GraphIDChain: []resource.GraphID{{Kind: resource.ItemKindDraft, ID: "draft+1"}},
	}
	routed.Claims = &claims
	return graph.WithRoutedTarget(ctx, routed), codec, target
}
