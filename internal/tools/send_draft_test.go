package tools

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/desek/outlook-local-mcp/internal/auth"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"
)

// TestSendDraftRequiresAcceptedElicitation verifies decline and unsupported
// clients never reach the irreversible Graph send seam.
func TestSendDraftRequiresAcceptedElicitation(t *testing.T) {
	for _, test := range []struct {
		name   string
		elicit func(context.Context, mcp.ElicitationRequest) (*mcp.ElicitationResult, error)
		error  bool
	}{
		{name: "declined", elicit: func(context.Context, mcp.ElicitationRequest) (*mcp.ElicitationResult, error) {
			return &mcp.ElicitationResult{ElicitationResponse: mcp.ElicitationResponse{Action: mcp.ElicitationResponseActionDecline}}, nil
		}},
		{name: "unsupported", error: true, elicit: func(context.Context, mcp.ElicitationRequest) (*mcp.ElicitationResult, error) {
			return nil, mcpserver.ErrElicitationNotSupported
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			sent := false
			state := &sendDraftState{
				load: func(context.Context, *msgraphsdk.GraphServiceClient, string) (draftSendSummary, error) {
					return draftSendSummary{Subject: "Review", Recipients: []string{"a@example.com"}, IsDraft: true}, nil
				},
				elicit: test.elicit,
				send:   func(context.Context, *msgraphsdk.GraphServiceClient, string) error { sent = true; return nil },
			}
			request := mcp.CallToolRequest{}
			request.Params.Arguments = map[string]any{"message_id": "draft-1"}
			ctx := auth.WithGraphClient(context.Background(), new(msgraphsdk.GraphServiceClient))
			result, err := handleSendDraft(state)(ctx, request)
			if err != nil || sent || result.IsError != test.error {
				t.Fatalf("result=%v err=%v sent=%v", result, err, sent)
			}
		})
	}
}

// TestSendDraftAcceptsOnlyExistingDraft verifies the accepted path sends once
// and reports Graph acceptance without claiming final delivery.
func TestSendDraftAcceptsOnlyExistingDraft(t *testing.T) {
	sendCount := 0
	state := &sendDraftState{
		load: func(context.Context, *msgraphsdk.GraphServiceClient, string) (draftSendSummary, error) {
			return draftSendSummary{Subject: "Review", Recipients: []string{"a@example.com"}, Attachments: []string{"report.pdf"}, IsDraft: true}, nil
		},
		elicit: func(_ context.Context, request mcp.ElicitationRequest) (*mcp.ElicitationResult, error) {
			if !strings.Contains(request.Params.Message, "report.pdf") || !strings.Contains(request.Params.Message, "a@example.com") {
				return nil, errors.New("review metadata missing")
			}
			return &mcp.ElicitationResult{ElicitationResponse: mcp.ElicitationResponse{Action: mcp.ElicitationResponseActionAccept, Content: map[string]any{"confirmed": true}}}, nil
		},
		send: func(context.Context, *msgraphsdk.GraphServiceClient, string) error { sendCount++; return nil },
	}
	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]any{"message_id": "draft-1"}
	ctx := auth.WithGraphClient(context.Background(), new(msgraphsdk.GraphServiceClient))
	result, err := handleSendDraft(state)(ctx, request)
	if err != nil || result.IsError || sendCount != 1 {
		t.Fatalf("result=%v err=%v sendCount=%d", result, err, sendCount)
	}
	text := extractText(t, result)
	if !strings.Contains(text, "accepted by Microsoft Graph") || !strings.Contains(text, "subject to Exchange processing") {
		t.Fatalf("unexpected confirmation: %q", text)
	}
}

// TestSendDraftRejectsChangedVersion verifies a draft edit after elicitation
// invalidates the human decision and prevents the irreversible send call.
func TestSendDraftRejectsChangedVersion(t *testing.T) {
	loadCount := 0
	sent := false
	state := &sendDraftState{
		load: func(context.Context, *msgraphsdk.GraphServiceClient, string) (draftSendSummary, error) {
			loadCount++
			return draftSendSummary{ChangeKey: string(rune('a' + loadCount)), Subject: "Review", IsDraft: true}, nil
		},
		elicit: func(context.Context, mcp.ElicitationRequest) (*mcp.ElicitationResult, error) {
			return &mcp.ElicitationResult{ElicitationResponse: mcp.ElicitationResponse{Action: mcp.ElicitationResponseActionAccept, Content: map[string]any{"confirmed": true}}}, nil
		},
		send: func(context.Context, *msgraphsdk.GraphServiceClient, string) error { sent = true; return nil },
	}
	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]any{"message_id": "draft-1"}
	ctx := auth.WithGraphClient(context.Background(), new(msgraphsdk.GraphServiceClient))
	result, err := handleSendDraft(state)(ctx, request)
	if err != nil || !result.IsError || sent {
		t.Fatalf("result=%v err=%v sent=%v", result, err, sent)
	}
}
