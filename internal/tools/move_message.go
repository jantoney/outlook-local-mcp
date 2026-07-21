package tools

import (
	"context"
	"fmt"
	"time"

	"github.com/desek/outlook-local-mcp/internal/graph"
	"github.com/desek/outlook-local-mcp/internal/resource"
	"github.com/mark3labs/mcp-go/mcp"
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
		return moveMessageToSemanticDestination(ctx, target, sourceID, destinationID, "Message moved to ordinary folder", timeout, codec)
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
