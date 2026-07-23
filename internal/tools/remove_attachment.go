package tools

import (
	"context"
	"fmt"
	"time"

	"github.com/desek/outlook-local-mcp/internal/graph"
	"github.com/desek/outlook-local-mcp/internal/logging"
	"github.com/desek/outlook-local-mcp/internal/resource"
	"github.com/desek/outlook-local-mcp/internal/validate"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/microsoftgraph/msgraph-sdk-go/users"
)

// NewHandleRemoveAttachment creates a draft-only attachment removal handler.
// The retry configuration applies only to the read-only draft preflight; the
// destructive DELETE is sent once with SDK retries disabled. The optional
// codec verifies shared attachment provenance and renews shared draft output.
// retryCfg and timeout control the preflight and request deadline; codecs may
// contain the shared reference signer. The returned MCP handler reports input,
// authorization, draft-state, and Graph failures as tool errors. Side effects
// include permanently removing one attachment from Graph.
func NewHandleRemoveAttachment(retryCfg graph.RetryConfig, timeout time.Duration, codecs ...*resource.ReferenceCodec) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		logger := logging.Logger(ctx)
		target, err := mailTargetFromContext(ctx)
		if err != nil {
			return mcp.NewToolResultError("no account selected"), nil
		}
		draftID, err := target.draftID(request.GetString("message_id", ""))
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		attachmentID, err := target.draftAttachmentID(
			request.GetString("attachment_id", ""),
			request.GetString("attachment_ref", ""),
			referenceCodec(codecs),
		)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if err := validate.ValidateResourceID(draftID, "message_id"); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if err := validate.ValidateResourceID(attachmentID, "attachment_id"); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if result := verifyRoutedIsDraft(ctx, target, retryCfg, timeout, draftID, logger); result != nil {
			return result, nil
		}
		if err := revalidateMailTarget(ctx, target); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		deleteCtx, cancel := graph.WithTimeout(ctx, timeout)
		config := &users.ItemMessagesItemAttachmentsAttachmentItemRequestBuilderDeleteRequestConfiguration{
			Options: graph.NoRetryRequestOptions(),
		}
		err = target.root.Messages().ByMessageId(draftID).Attachments().ByAttachmentId(attachmentID).Delete(deleteCtx, config)
		cancel()
		if err != nil {
			logger.ErrorContext(ctx, "remove draft attachment failed", "error", graph.FormatGraphError(err))
			return mcp.NewToolResultError(attachmentRemovalError(target, err)), nil
		}

		logger.InfoContext(ctx, "draft attachment removed", "draft_id", draftID, "attachment_id", attachmentID)
		response := fmt.Sprintf("Attachment removed from draft.\nDraft ID: %s\nAttachment ID: %s", draftID, attachmentID)
		if line := AccountInfoLine(ctx); line != "" {
			response += "\n" + line
		}
		confirmed := response
		response, err = sharedAttachmentConfirmation(confirmed, target, referenceCodec(codecs), draftID)
		if err != nil {
			recordDraftPartialSuccess(ctx)
			partial := confirmed + "\nPARTIAL SUCCESS: attachment removal completed, but renewed draft provenance is unavailable: " + err.Error()
			if target.isShared() {
				partial += fmt.Sprintf("\nShared Target: %s (%s)", target.target.Alias, target.target.Owner)
			}
			return mcp.NewToolResultText(partial), nil
		}
		return mcp.NewToolResultText(response), nil
	}
}

// attachmentRemovalError distinguishes an ordinary Graph rejection from an
// ambiguous transport or server failure. Callers must inspect the draft after
// an ambiguous result because the destructive request is never retried. The
// target controls route-aware redaction and err supplies the Graph failure;
// the returned string is safe for an MCP error response and has no side effects.
func attachmentRemovalError(target mailReadTarget, err error) string {
	status := graph.ExtractHTTPStatus(err)
	if status == 0 || status >= 500 {
		return "attachment removal outcome is uncertain; inspect the draft before retrying or adding another attachment. Detail: " + target.graphError(err)
	}
	return target.graphError(err)
}
