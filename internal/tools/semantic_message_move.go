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

// moveMessageToSemanticDestination performs one non-retried Graph move to a
// destination selected by server semantics rather than caller input. It
// returns Graph's replacement message identity and records partial completion
// when the committed result cannot be referenced.
func moveMessageToSemanticDestination(ctx context.Context, target mailReadTarget, sourceID, destinationID, action string, timeout time.Duration, codec *resource.ReferenceCodec) (*mcp.CallToolResult, error) {
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
	if moved == nil || graph.SafeStr(moved.GetId()) == "" {
		recordDraftPartialSuccess(ctx)
		return mcp.NewToolResultText("PARTIAL SUCCESS: Graph accepted the " + action + " but returned no destination message ID. Do not repeat it; inspect the mailbox before continuing."), nil
	}
	movedID := graph.SafeStr(moved.GetId())
	reference, err := signMailItem(codec, target.target, resource.ItemKindMessage, movedID)
	if err != nil {
		recordDraftPartialSuccess(ctx)
		return mcp.NewToolResultText(fmt.Sprintf("PARTIAL SUCCESS: %s completed, but the new reference could not be signed.\nDestination Message ID: %s\nDo not repeat the operation; inspect the mailbox.\nCompletion detail: %s", action, movedID, err.Error())), nil
	}
	response := fmt.Sprintf("%s completed.\nDestination Message ID: %s\nResource Ref: %s", action, movedID, reference)
	if target.isShared() {
		response += fmt.Sprintf("\nShared Target: %s (%s)", target.target.Alias, target.target.Owner)
	}
	if line := AccountInfoLine(ctx); line != "" {
		response += "\n" + line
	}
	return mcp.NewToolResultText(response), nil
}
