// Package tools provides MCP tool definitions and handler constructors for the
// Outlook Calendar MCP Server.
//
// This file provides the mail_create_forward_draft MCP tool, which creates a
// forward draft via POST /me/messages/{id}/createForward on the Microsoft
// Graph API. The draft is not sent: it is left in the user's Drafts folder
// for review and manual send.
package tools

import (
	"context"
	"time"

	"github.com/desek/outlook-local-mcp/internal/graph"
	"github.com/desek/outlook-local-mcp/internal/logging"
	"github.com/desek/outlook-local-mcp/internal/resource"
	"github.com/desek/outlook-local-mcp/internal/validate"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/microsoftgraph/msgraph-sdk-go/models"
	"github.com/microsoftgraph/msgraph-sdk-go/users"
)

// NewCreateForwardDraftTool creates the MCP tool definition for
// mail_create_forward_draft. It requires a message_id identifying the source
// message, and accepts optional to_recipients (comma-separated) and comment
// parameters.
//
// Returns the configured mcp.Tool ready for registration with server.AddTool.
func NewCreateForwardDraftTool() mcp.Tool {
	return mcp.NewTool("mail_create_forward_draft",
		mcp.WithTitleAnnotation("Create Email Forward Draft"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithDescription(
			"Create a forward draft of an existing message. The draft is NOT sent "+
				"automatically: it appears in the user's Outlook Drafts folder for "+
				"review, edit, and manual send.",
		),
		mcp.WithString("message_id", mcp.Required(),
			mcp.Description("The unique identifier of the source message to forward."),
		),
		mcp.WithString("to_recipients",
			mcp.Description("Optional comma-separated list of To recipient email addresses."),
		),
		mcp.WithString("comment",
			mcp.Description("Optional forward body text prepended to the quoted original message."),
		),
		mcp.WithString("account",
			mcp.Description(AccountParamDescription),
		),
	)
}

// NewHandleCreateForwardDraft creates the MCP tool handler for
// mail_create_forward_draft. It calls the Graph SDK's createForward action
// with optional recipients and comment. When provenancePropertyID is
// non-empty, a follow-up PATCH stamps the provenance extended property on
// the returned draft.
//
// Parameters:
//   - retryCfg: retry configuration for the idempotent provenance PATCH only;
//     the non-idempotent create action is attempted once.
//   - timeout: the maximum duration for a single Graph API call.
//   - provenancePropertyID: the full MAPI property ID for provenance tagging.
//     Empty string disables the follow-up PATCH.
//   - referenceCodec: signs shared-mail draft references; it may be nil only
//     for own-mail handlers.
//
// Returns a handler function compatible with the MCP server AddTool signature.
//
// Side effects: calls createForward on the exact routed source message, then
// optionally PATCHes that same mailbox's created draft for provenance.
func NewHandleCreateForwardDraft(retryCfg graph.RetryConfig, timeout time.Duration, provenancePropertyID string, referenceCodec *resource.ReferenceCodec) func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		logger := logging.Logger(ctx)

		target, err := mailTargetFromContext(ctx)
		if err != nil {
			return mcp.NewToolResultError("no account selected"), nil
		}

		messageID, err := target.messageID(request.GetString("message_id", ""))
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if err := validate.ValidateResourceID(messageID, "message_id"); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		args := request.GetArguments()
		comment, _ := args["comment"].(string)
		toRaw, _ := args["to_recipients"].(string)
		toAddrs, err := validate.ValidateRecipients(toRaw, "to_recipients")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		body := users.NewItemMessagesItemCreateForwardPostRequestBody()
		if comment != "" {
			c := comment
			body.SetComment(&c)
		}
		if len(toAddrs) > 0 {
			body.SetToRecipients(BuildRecipients(toAddrs))
		}

		timeoutCtx, cancel := graph.WithTimeout(ctx, timeout)
		defer cancel()
		if err := revalidateMailTarget(ctx, target); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		// createForward is non-idempotent and is attempted exactly once.
		config := &users.ItemMessagesItemCreateForwardRequestBuilderPostRequestConfiguration{Options: graph.NoRetryRequestOptions()}
		created, err := target.root.Messages().ByMessageId(messageID).CreateForward().Post(timeoutCtx, body, config)
		if err != nil {
			logger.ErrorContext(ctx, "create forward draft failed", "error", graph.FormatGraphError(err))
			return mcp.NewToolResultError(draftCreateError(target, err)), nil
		}

		draftID := graph.SafeStr(created.GetId())
		draftSubject := graph.SafeStr(created.GetSubject())

		if provenancePropertyID != "" && draftID != "" {
			patch := models.NewMessage()
			MaybeSetMailProvenance(patch, provenancePropertyID)
			patchCtx, patchCancel := graph.WithTimeout(ctx, timeout)
			pErr := graph.RetryGraphCall(ctx, retryCfg, func() error {
				_, e := target.root.Messages().ByMessageId(draftID).Patch(patchCtx, patch, nil)
				return e
			})
			patchCancel()
			if pErr != nil {
				logger.WarnContext(ctx, "provenance patch failed", "draft_id", draftID, "error", graph.FormatGraphError(pErr))
				response := draftCompletionPartial("Forward", draftSubject, draftID, target.graphError(pErr), target, referenceCodec)
				recordDraftPartialSuccess(ctx)
				if line := AccountInfoLine(ctx); line != "" {
					response += "\n" + line
				}
				return mcp.NewToolResultText(response), nil
			}
		}

		logger.InfoContext(ctx, "forward draft created", "draft_id", draftID)

		response, err := draftWriteConfirmation("created", draftSubject, draftID, target, referenceCodec)
		if err != nil {
			response = draftCompletionPartial("Forward", draftSubject, draftID, err.Error(), target, referenceCodec)
			recordDraftPartialSuccess(ctx)
		}
		if line := AccountInfoLine(ctx); line != "" {
			response += "\n" + line
		}
		return mcp.NewToolResultText(response), nil
	}
}
