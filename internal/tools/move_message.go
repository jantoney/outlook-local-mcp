package tools

import (
	"context"
	"fmt"
	"time"

	"github.com/desek/outlook-local-mcp/internal/graph"
	"github.com/desek/outlook-local-mcp/internal/resource"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/microsoftgraph/msgraph-sdk-go/users"
)

// NewHandleMoveMessage creates an ordinary-folder move handler. Both source
// and destination must be signed references for the same immutable mailbox.
// The Graph move is attempted once because its response supplies a new ID and
// an ambiguous mutation must not be retried.
func NewHandleMoveMessage(retryCfg graph.RetryConfig, timeout time.Duration, codec *resource.ReferenceCodec) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		target, err := mailTargetFromContext(ctx)
		if err != nil {
			return mcp.NewToolResultError("no account selected"), nil
		}
		sourceID, err := target.referencedMessageID()
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		destinationID, err := ordinaryDestinationID(target, request.GetString("destination_folder_ref", ""), codec)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		resolveCtx, resolveCancel := graph.WithTimeout(ctx, timeout)
		var resolvedID string
		err = graph.RetryGraphCall(ctx, retryCfg, func() error {
			folder, callErr := target.root.MailFolders().ByMailFolderId(destinationID).Get(resolveCtx, nil)
			if folder != nil {
				resolvedID = graph.SafeStr(folder.GetId())
			}
			return callErr
		})
		resolveCancel()
		if err != nil {
			return mcp.NewToolResultError(target.graphError(err)), nil
		}
		if resolvedID != destinationID {
			return mcp.NewToolResultError("destination folder reference no longer resolves to the same folder; no move was started"), nil
		}
		if err := revalidateMailTarget(ctx, target); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		body := users.NewItemMessagesItemMovePostRequestBody()
		body.SetDestinationId(&destinationID)
		timeoutCtx, cancel := graph.WithTimeout(ctx, timeout)
		defer cancel()
		moved, err := target.root.Messages().ByMessageId(sourceID).Move().Post(timeoutCtx, body, nil)
		if err != nil {
			return mcp.NewToolResultError(moveMutationError(target, err)), nil
		}
		movedID := graph.SafeStr(moved.GetId())
		if movedID == "" {
			recordDraftPartialSuccess(ctx)
			return mcp.NewToolResultText("PARTIAL SUCCESS: Graph accepted the move but returned no destination message ID. Do not repeat the move; inspect the destination folder before continuing."), nil
		}
		reference, err := signMailItem(codec, target.target, resource.ItemKindMessage, movedID)
		if err != nil {
			recordDraftPartialSuccess(ctx)
			return mcp.NewToolResultText(fmt.Sprintf("PARTIAL SUCCESS: message moved to the ordinary destination, but the new reference could not be signed.\nDestination Message ID: %s\nDo not repeat the move; inspect the destination folder.\nCompletion detail: %s", movedID, err.Error())), nil
		}
		response := fmt.Sprintf("Message moved to ordinary folder.\nDestination Message ID: %s\nResource Ref: %s", movedID, reference)
		if target.isShared() {
			response += fmt.Sprintf("\nShared Target: %s (%s)", target.target.Alias, target.target.Owner)
		}
		if line := AccountInfoLine(ctx); line != "" {
			response += "\n" + line
		}
		return mcp.NewToolResultText(response), nil
	}
}

// ordinaryDestinationID verifies a signed, same-target folder classification
// and returns its opaque Graph ID without making a Graph request.
func ordinaryDestinationID(target mailReadTarget, reference string, codec *resource.ReferenceCodec) (string, error) {
	if reference == "" || codec == nil {
		return "", fmt.Errorf("destination_folder_ref is required")
	}
	claims, err := codec.Verify(reference)
	if err != nil {
		return "", err
	}
	if claims.AccountID != target.target.AccountID || claims.ResourceID != target.target.ResourceID ||
		claims.ResourceKind != target.target.Kind || claims.MailboxView != target.target.View {
		return "", fmt.Errorf("destination folder reference does not match the resolved mailbox")
	}
	if claims.ItemKind != resource.ItemKindMailFolder || len(claims.GraphIDChain) != 1 {
		return "", fmt.Errorf("destination_folder_ref must identify one mail folder")
	}
	if claims.MailFolderClass != resource.MailFolderClassOrdinary {
		return "", fmt.Errorf("destination folder is reserved or unclassified and cannot bypass its exact action gate")
	}
	return claims.GraphIDChain[0].ID, nil
}

// moveMutationError labels ambiguous one-attempt move failures as uncertain.
func moveMutationError(target mailReadTarget, err error) string {
	status := graph.ExtractHTTPStatus(err)
	if status == 0 || status >= 500 {
		return "message move outcome is uncertain; do not repeat the move until both source and destination folders are inspected. Detail: " + target.graphError(err)
	}
	return target.graphError(err)
}
