package tools

import (
	"context"
	"fmt"
	"time"

	"github.com/desek/outlook-local-mcp/internal/graph"
	"github.com/desek/outlook-local-mcp/internal/resource"
	"github.com/mark3labs/mcp-go/mcp"
)

// NewHandleRestoreMessage creates a handler that restores a referenced message
// only when Graph currently reports it in Deleted Items and the caller supplies
// a same-target ordinary destination reference.
func NewHandleRestoreMessage(retryCfg graph.RetryConfig, timeout time.Duration, codec *resource.ReferenceCodec) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
		readCtx, cancel := graph.WithTimeout(ctx, timeout)
		defer cancel()
		var parentID, deletedItemsID, resolvedDestinationID string
		if err := graph.RetryGraphCall(ctx, retryCfg, func() error {
			message, callErr := target.root.Messages().ByMessageId(sourceID).Get(readCtx, nil)
			if message != nil {
				parentID = graph.SafeStr(message.GetParentFolderId())
			}
			return callErr
		}); err != nil {
			return mcp.NewToolResultError(target.graphError(err)), nil
		}
		if err := graph.RetryGraphCall(ctx, retryCfg, func() error {
			folder, callErr := target.root.MailFolders().ByMailFolderId("deleteditems").Get(readCtx, nil)
			if folder != nil {
				deletedItemsID = graph.SafeStr(folder.GetId())
			}
			return callErr
		}); err != nil {
			return mcp.NewToolResultError(target.graphError(err)), nil
		}
		if parentID == "" || deletedItemsID == "" || parentID != deletedItemsID {
			return mcp.NewToolResultError("restore requires a message currently located in Deleted Items; recoverable-items restoration is not supported"), nil
		}
		if err := graph.RetryGraphCall(ctx, retryCfg, func() error {
			folder, callErr := target.root.MailFolders().ByMailFolderId(destinationID).Get(readCtx, nil)
			if folder != nil {
				resolvedDestinationID = graph.SafeStr(folder.GetId())
			}
			return callErr
		}); err != nil {
			return mcp.NewToolResultError(target.graphError(err)), nil
		}
		if resolvedDestinationID != destinationID {
			return mcp.NewToolResultError(fmt.Sprintf("destination folder %q no longer resolves; no restore was started", destinationID)), nil
		}
		return moveMessageToSemanticDestination(ctx, target, sourceID, destinationID, "Message restored from Deleted Items", timeout, codec)
	}
}
