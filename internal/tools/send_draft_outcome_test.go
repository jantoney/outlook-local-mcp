package tools

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/desek/outlook-local-mcp/internal/auth"
	"github.com/mark3labs/mcp-go/mcp"
	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"
	"github.com/microsoftgraph/msgraph-sdk-go/models/odataerrors"
)

// TestOwnSendDraftClassifiesAttemptedOutcomes verifies ambiguous send errors
// require reconciliation while definite client errors remain denied.
func TestOwnSendDraftClassifiesAttemptedOutcomes(t *testing.T) {
	statusError := func(status int) error {
		err := odataerrors.NewODataError()
		err.ResponseStatusCode = status
		return err
	}
	tests := []struct {
		name      string
		sendErr   error
		outcome   string
		uncertain bool
	}{
		{name: "transport", sendErr: errors.New("connection reset"), outcome: "uncertain", uncertain: true},
		{name: "timeout", sendErr: context.DeadlineExceeded, outcome: "uncertain", uncertain: true},
		{name: "throttled", sendErr: statusError(http.StatusTooManyRequests), outcome: "uncertain", uncertain: true},
		{name: "server", sendErr: statusError(http.StatusServiceUnavailable), outcome: "uncertain", uncertain: true},
		{name: "gateway", sendErr: statusError(http.StatusGatewayTimeout), outcome: "uncertain", uncertain: true},
		{name: "definite", sendErr: statusError(http.StatusBadRequest), outcome: "denied"},
		{name: "accepted", outcome: "accepted"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state := acceptedOwnSendState(test.sendErr)
			ctx := auth.WithGraphClient(context.Background(), new(msgraphsdk.GraphServiceClient))
			ctx = auth.WithAuditOutcomeRecorder(ctx)
			result, err := handleSendDraft(state)(ctx, ownSendRequest())
			if err != nil || result.IsError != (test.sendErr != nil) {
				t.Fatalf("result = %+v, error = %v", result, err)
			}
			outcome, ok := auth.AuditOutcomeFromContext(ctx)
			if !ok || outcome != test.outcome {
				t.Fatalf("audit outcome = %q, %t; want %q", outcome, ok, test.outcome)
			}
			text := result.Content[0].(mcp.TextContent).Text
			if test.uncertain && (!strings.Contains(text, "outcome is uncertain") || !strings.Contains(text, "Drafts and Sent Items") || !strings.Contains(text, "do not retry")) {
				t.Fatalf("uncertain send lacks reconciliation guidance: %s", text)
			}
			if !test.uncertain && strings.Contains(text, "outcome is uncertain") {
				t.Fatalf("definite outcome was classified uncertain: %s", text)
			}
		})
	}
}

// TestSendOwnDraftOnceDoesNotDispatchCanceledContext verifies cancellation
// observed before the SDK call remains an ordinary no-attempt timeout.
func TestSendOwnDraftOnceDoesNotDispatchCanceledContext(t *testing.T) {
	var calls int
	client, server := newRealSDKGraphClient(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls++ }))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := sendOwnDraftOnce(ctx, client, "draft-1", time.Second)
	if !errors.Is(err, errOwnSendNotStarted) || calls != 0 {
		t.Fatalf("error = %v, calls = %d; want not-started and zero", err, calls)
	}
}

// TestOwnSendDraftCanceledBeforeDispatchIsNotUncertain verifies the handler
// records a known canceled outcome and never invokes its send seam.
func TestOwnSendDraftCanceledBeforeDispatchIsNotUncertain(t *testing.T) {
	sent := false
	state := acceptedOwnSendState(nil)
	state.send = func(context.Context, *msgraphsdk.GraphServiceClient, string) error {
		sent = true
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	ctx = auth.WithGraphClient(ctx, new(msgraphsdk.GraphServiceClient))
	ctx = auth.WithAuditOutcomeRecorder(ctx)
	result, err := handleSendDraft(state)(ctx, ownSendRequest())
	if err != nil || !result.IsError || sent {
		t.Fatalf("result = %+v, sent = %t, error = %v", result, sent, err)
	}
	text := result.Content[0].(mcp.TextContent).Text
	if strings.Contains(text, "outcome is uncertain") || !strings.Contains(text, "before send dispatch") {
		t.Fatalf("pre-dispatch cancellation = %s", text)
	}
	if outcome, ok := auth.AuditOutcomeFromContext(ctx); !ok || outcome != "canceled" {
		t.Fatalf("audit outcome = %q, %t; want canceled", outcome, ok)
	}
}

// TestSendOwnDraftOnceAttemptsTransientStatusesOnce verifies the SDK cannot
// transparently repeat an ambiguous own-mail send POST.
func TestSendOwnDraftOnceAttemptsTransientStatusesOnce(t *testing.T) {
	for _, status := range []int{http.StatusTooManyRequests, http.StatusServiceUnavailable, http.StatusGatewayTimeout} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var calls int
			client, server := newRealSDKGraphClient(t, transientMutationHandler(&calls, status, nil))
			defer server.Close()
			_ = sendOwnDraftOnce(context.Background(), client, "draft-1", time.Second)
			if calls != 1 {
				t.Fatalf("send calls = %d, want one after HTTP %d", calls, status)
			}
		})
	}
}

// acceptedOwnSendState returns an accepted, unchanged review whose send seam
// produces sendErr.
func acceptedOwnSendState(sendErr error) *sendDraftState {
	return &sendDraftState{
		load: func(context.Context, *msgraphsdk.GraphServiceClient, string) (draftSendSummary, error) {
			return draftSendSummary{ChangeKey: "v1", Subject: "Review", Recipients: []string{"a@example.com"}, IsDraft: true}, nil
		},
		elicit: func(context.Context, mcp.ElicitationRequest) (*mcp.ElicitationResult, error) {
			return &mcp.ElicitationResult{ElicitationResponse: mcp.ElicitationResponse{Action: mcp.ElicitationResponseActionAccept, Content: map[string]any{"confirmed": true}}}, nil
		},
		send: func(context.Context, *msgraphsdk.GraphServiceClient, string) error { return sendErr },
	}
}

// ownSendRequest returns a valid own-mail send request.
func ownSendRequest() mcp.CallToolRequest {
	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]any{"message_id": "draft-1"}
	return request
}
