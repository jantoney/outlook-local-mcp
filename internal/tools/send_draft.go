package tools

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/desek/outlook-local-mcp/internal/graph"
	"github.com/desek/outlook-local-mcp/internal/validate"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"
	"github.com/microsoftgraph/msgraph-sdk-go/models"
)

// draftSendSummary is the immutable human-review snapshot fetched immediately
// before elicitation and used only to describe the pending send.
type draftSendSummary struct {
	ChangeKey   string
	Subject     string
	Recipients  []string
	Attachments []string
	IsDraft     bool
}

// sendDraftState isolates Graph reads, human elicitation, and the irreversible
// send call so the fail-closed control flow can be tested without a network.
type sendDraftState struct {
	load   func(context.Context, *msgraphsdk.GraphServiceClient, string) (draftSendSummary, error)
	elicit func(context.Context, mcp.ElicitationRequest) (*mcp.ElicitationResult, error)
	send   func(context.Context, *msgraphsdk.GraphServiceClient, string) error
}

// NewHandleSendDraft creates a handler that sends an existing draft only after
// an MCP client presents its metadata and a human explicitly accepts.
func NewHandleSendDraft(retryCfg graph.RetryConfig, timeout time.Duration) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	state := &sendDraftState{
		load: func(ctx context.Context, client *msgraphsdk.GraphServiceClient, messageID string) (draftSendSummary, error) {
			return loadDraftSendSummary(ctx, client, retryCfg, timeout, messageID)
		},
		elicit: defaultSendDraftElicit,
		send: func(ctx context.Context, client *msgraphsdk.GraphServiceClient, messageID string) error {
			timeoutCtx, cancel := graph.WithTimeout(ctx, timeout)
			defer cancel()
			// Sending is irreversible and non-idempotent. Never blindly retry an
			// ambiguous failure because Graph may already have accepted the send.
			return client.Me().Messages().ByMessageId(messageID).Send().Post(timeoutCtx, nil)
		},
	}
	return handleSendDraft(state)
}

// handleSendDraft implements validation and the mandatory accept-only gate.
func handleSendDraft(state *sendDraftState) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := GraphClient(ctx)
		if err != nil {
			return mcp.NewToolResultError("no account selected"), nil
		}
		messageID, err := request.RequireString("message_id")
		if err != nil || messageID == "" {
			return mcp.NewToolResultError("missing required parameter: message_id"), nil
		}
		if err := validate.ValidateResourceID(messageID, "message_id"); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		summary, err := state.load(ctx, client, messageID)
		if err != nil {
			return mcp.NewToolResultError(graph.RedactGraphError(err)), nil
		}
		if !summary.IsDraft {
			return mcp.NewToolResultError("message is not a draft; refusing to send"), nil
		}
		result, err := state.elicit(ctx, sendDraftElicitation(summary))
		if err != nil {
			return mcp.NewToolResultError("send requires human confirmation through MCP elicitation; this client does not support it. Send the draft through Outlook instead"), nil
		}
		if result == nil || result.Action != mcp.ElicitationResponseActionAccept || !elicitationConfirmed(result.Content) {
			return mcp.NewToolResultText("Draft send cancelled; no message was sent."), nil
		}
		current, err := state.load(ctx, client, messageID)
		if err != nil {
			return mcp.NewToolResultError(graph.RedactGraphError(err)), nil
		}
		if !sameDraftVersion(summary, current) {
			return mcp.NewToolResultError("draft changed after confirmation; review and confirm it again"), nil
		}
		if err := state.send(ctx, client, messageID); err != nil {
			return mcp.NewToolResultError(graph.RedactGraphError(err)), nil
		}
		response := fmt.Sprintf("Draft send accepted by Microsoft Graph: %s\nFinal delivery remains subject to Exchange processing.", messageID)
		if line := AccountInfoLine(ctx); line != "" {
			response += "\n" + line
		}
		return mcp.NewToolResultText(response), nil
	}
}

// defaultSendDraftElicit requests form elicitation from the current MCP server.
func defaultSendDraftElicit(ctx context.Context, request mcp.ElicitationRequest) (*mcp.ElicitationResult, error) {
	server := mcpserver.ServerFromContext(ctx)
	if server == nil || mcpserver.ClientSessionFromContext(ctx) == nil {
		return nil, mcpserver.ErrElicitationNotSupported
	}
	return server.RequestElicitation(ctx, request)
}

// sendDraftElicitation builds the non-agent-bypassable human review prompt.
func sendDraftElicitation(summary draftSendSummary) mcp.ElicitationRequest {
	message := fmt.Sprintf("Send this draft?\nSubject: %s\nRecipients: %s\nAttachments: %s",
		summary.Subject, joinedOrNone(summary.Recipients), joinedOrNone(summary.Attachments))
	return mcp.ElicitationRequest{Params: mcp.ElicitationParams{
		Message: message,
		RequestedSchema: map[string]any{"type": "object", "properties": map[string]any{
			"confirmed": map[string]any{"type": "boolean", "description": "Confirm sending this exact draft."},
		}, "required": []string{"confirmed"}},
	}}
}

// elicitationConfirmed accepts only the explicit checked confirmation returned
// inside the human elicitation response; an accepted empty form is insufficient.
func elicitationConfirmed(content any) bool {
	values, ok := content.(map[string]any)
	if !ok {
		return false
	}
	confirmed, ok := values["confirmed"].(bool)
	return ok && confirmed
}

// loadDraftSendSummary fetches the draft and attachment names shown to the
// human. Both reads complete before elicitation; the upload URL is never used.
func loadDraftSendSummary(ctx context.Context, client *msgraphsdk.GraphServiceClient, retryCfg graph.RetryConfig, timeout time.Duration, messageID string) (draftSendSummary, error) {
	timeoutCtx, cancel := graph.WithTimeout(ctx, timeout)
	defer cancel()
	var message models.Messageable
	if err := graph.RetryGraphCall(ctx, retryCfg, func() error {
		var callErr error
		message, callErr = client.Me().Messages().ByMessageId(messageID).Get(timeoutCtx, nil)
		return callErr
	}); err != nil {
		return draftSendSummary{}, err
	}
	summary := draftSendSummary{ChangeKey: graph.SafeStr(message.GetChangeKey()), Subject: graph.SafeStr(message.GetSubject()), IsDraft: graph.SafeBool(message.GetIsDraft())}
	summary.Recipients = append(summary.Recipients, recipientAddresses(message.GetToRecipients())...)
	summary.Recipients = append(summary.Recipients, recipientAddresses(message.GetCcRecipients())...)
	summary.Recipients = append(summary.Recipients, recipientAddresses(message.GetBccRecipients())...)
	var collection models.AttachmentCollectionResponseable
	if err := graph.RetryGraphCall(ctx, retryCfg, func() error {
		var callErr error
		collection, callErr = client.Me().Messages().ByMessageId(messageID).Attachments().Get(timeoutCtx, nil)
		return callErr
	}); err != nil {
		return draftSendSummary{}, err
	}
	if collection != nil {
		for _, attachment := range collection.GetValue() {
			summary.Attachments = append(summary.Attachments, graph.SafeStr(attachment.GetName()))
		}
	}
	return summary, nil
}

// sameDraftVersion reports whether the post-confirmation draft is the exact
// version the human reviewed. Graph change keys are authoritative; the field
// comparison is a defensive fallback for test doubles or incomplete responses.
func sameDraftVersion(reviewed, current draftSendSummary) bool {
	if !current.IsDraft {
		return false
	}
	if reviewed.ChangeKey != "" || current.ChangeKey != "" {
		return reviewed.ChangeKey != "" && reviewed.ChangeKey == current.ChangeKey
	}
	return reviewed.Subject == current.Subject &&
		slices.Equal(reviewed.Recipients, current.Recipients) &&
		slices.Equal(reviewed.Attachments, current.Attachments)
}

// recipientAddresses returns displayable addresses from Graph recipients.
func recipientAddresses(recipients []models.Recipientable) []string {
	addresses := make([]string, 0, len(recipients))
	for _, recipient := range recipients {
		if recipient == nil || recipient.GetEmailAddress() == nil {
			continue
		}
		address := graph.SafeStr(recipient.GetEmailAddress().GetAddress())
		if address != "" {
			addresses = append(addresses, address)
		}
	}
	return addresses
}

// joinedOrNone formats review lists without leaving ambiguous empty fields.
func joinedOrNone(values []string) string {
	if len(values) == 0 {
		return "(none)"
	}
	return strings.Join(values, ", ")
}
