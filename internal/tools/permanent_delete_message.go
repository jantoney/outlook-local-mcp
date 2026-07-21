package tools

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"time"

	"github.com/desek/outlook-local-mcp/internal/auth"
	"github.com/desek/outlook-local-mcp/internal/graph"
	"github.com/desek/outlook-local-mcp/internal/logging"
	"github.com/mark3labs/mcp-go/mcp"
)

// permanentDeleteSummary is the immutable message snapshot presented for
// human review and compared after elicitation to reject stale confirmation.
type permanentDeleteSummary struct {
	ChangeKey string
	Subject   string
}

// permanentDeleteState isolates Graph reads, human elicitation, and the one
// destructive request so fail-closed behavior can be tested without a client.
type permanentDeleteState struct {
	load   func(context.Context, mailReadTarget, string) (permanentDeleteSummary, error)
	elicit func(context.Context, mcp.ElicitationRequest) (*mcp.ElicitationResult, error)
	delete func(context.Context, mailReadTarget, string) error
}

// NewHandlePermanentDeleteMessage creates a handler that permanently deletes
// exactly one referenced message only after fresh MCP human confirmation. The
// Graph action is attempted once and ambiguous failures are never retried.
func NewHandlePermanentDeleteMessage(retryCfg graph.RetryConfig, timeout time.Duration) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	state := &permanentDeleteState{
		load: func(ctx context.Context, target mailReadTarget, messageID string) (permanentDeleteSummary, error) {
			return loadPermanentDeleteSummary(ctx, target, retryCfg, timeout, messageID)
		},
		elicit: defaultSendDraftElicit,
		delete: func(ctx context.Context, target mailReadTarget, messageID string) error {
			timeoutCtx, cancel := graph.WithTimeout(ctx, timeout)
			defer cancel()
			return target.root.Messages().ByMessageId(messageID).PermanentDelete().Post(timeoutCtx, nil)
		},
	}
	return handlePermanentDeleteMessage(state)
}

// handlePermanentDeleteMessage implements the accept-only, version-bound,
// target-revalidated destructive flow.
func handlePermanentDeleteMessage(state *permanentDeleteState) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		target, err := mailTargetFromContext(ctx)
		if err != nil {
			return mcp.NewToolResultError("no account selected"), nil
		}
		messageID, err := target.referencedMessageID()
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		reviewed, err := state.load(ctx, target, messageID)
		if err != nil {
			return mcp.NewToolResultError(target.graphError(err)), nil
		}
		auth.RecordAuditConfirmation(ctx, "requested")
		result, err := state.elicit(ctx, permanentDeleteElicitation(ctx, target, request.GetString("message_ref", ""), reviewed))
		if err != nil {
			auth.RecordAuditConfirmation(ctx, "unsupported_or_failed")
			return mcp.NewToolResultError("permanent deletion requires fresh human confirmation through MCP elicitation; this client does not support it"), nil
		}
		if result == nil || result.Action != mcp.ElicitationResponseActionAccept || !elicitationConfirmed(result.Content) {
			auth.RecordAuditConfirmation(ctx, "declined_or_cancelled")
			auth.RecordAuditOutcome(ctx, "canceled")
			return mcp.NewToolResultText("Permanent deletion cancelled; no message was deleted."), nil
		}
		current, err := state.load(ctx, target, messageID)
		if err != nil {
			return mcp.NewToolResultError(target.graphError(err)), nil
		}
		if reviewed.ChangeKey == "" || current.ChangeKey == "" || reviewed.ChangeKey != current.ChangeKey {
			auth.RecordAuditConfirmation(ctx, "stale")
			return mcp.NewToolResultError("message changed after confirmation; inspect it and confirm permanent deletion again"), nil
		}
		if err := revalidateMailTarget(ctx, target); err != nil {
			auth.RecordAuditConfirmation(ctx, "authorization_changed")
			return mcp.NewToolResultError(err.Error()), nil
		}
		if err := state.delete(ctx, target, messageID); err != nil {
			auth.RecordAuditConfirmation(ctx, "accepted")
			status := graph.ExtractHTTPStatus(err)
			if status == 0 || status >= 500 {
				auth.RecordAuditOutcome(ctx, "uncertain")
				return mcp.NewToolResultError("permanent deletion outcome is uncertain; do not repeat it until mailbox inspection confirms whether the message remains. Detail: " + target.graphError(err)), nil
			}
			auth.RecordAuditOutcome(ctx, "denied")
			return mcp.NewToolResultError(target.graphError(err)), nil
		}
		auth.RecordAuditConfirmation(ctx, "accepted")
		auth.RecordAuditOutcome(ctx, "accepted")
		response := "Message permanently deleted through Microsoft Graph. Outlook clients cannot recover it; mailbox retention or legal hold may still apply."
		if line := AccountInfoLine(ctx); line != "" {
			response += "\n" + line
		}
		return mcp.NewToolResultText(response), nil
	}
}

// loadPermanentDeleteSummary fetches only the version and subject required for
// review. It performs read-only retry before the destructive stage.
func loadPermanentDeleteSummary(ctx context.Context, target mailReadTarget, retryCfg graph.RetryConfig, timeout time.Duration, messageID string) (permanentDeleteSummary, error) {
	timeoutCtx, cancel := graph.WithTimeout(ctx, timeout)
	defer cancel()
	var summary permanentDeleteSummary
	err := graph.RetryGraphCall(ctx, retryCfg, func() error {
		message, callErr := target.root.Messages().ByMessageId(messageID).Get(timeoutCtx, nil)
		if message != nil {
			summary = permanentDeleteSummary{ChangeKey: graph.SafeStr(message.GetChangeKey()), Subject: graph.SafeStr(message.GetSubject())}
		}
		return callErr
	})
	return summary, err
}

// permanentDeleteElicitation builds a fresh warning bound to the exact routed
// target, signed-reference fingerprint, and reviewed message version.
func permanentDeleteElicitation(ctx context.Context, target mailReadTarget, reference string, summary permanentDeleteSummary) mcp.ElicitationRequest {
	fingerprint := sha256.Sum256([]byte(reference))
	account := AccountInfoLine(ctx)
	if account == "" {
		account = "Account ID: " + string(target.target.AccountID)
	}
	targetLine := "Mailbox: own mailbox"
	if target.isShared() {
		targetLine = fmt.Sprintf("Shared mailbox: %s (%s)", target.target.Alias, logging.MaskEmail(target.target.Owner))
	}
	subject := strings.TrimSpace(summary.Subject)
	if subject == "" {
		subject = "(no subject)"
	}
	if len(subject) > 120 {
		subject = subject[:120] + "…"
	}
	message := fmt.Sprintf("PERMANENTLY DELETE this exact message?\n%s\n%s\nSubject: %s\nReference fingerprint: %x\n\nOutlook clients cannot recover this message. Retention or legal hold may still apply.", account, targetLine, subject, fingerprint[:8])
	return mcp.ElicitationRequest{Params: mcp.ElicitationParams{
		Message: message,
		RequestedSchema: map[string]any{"type": "object", "properties": map[string]any{
			"confirmed": map[string]any{"type": "boolean", "description": "Confirm permanent deletion of this exact reviewed message."},
		}, "required": []string{"confirmed"}},
	}}
}
