package tools

import (
	"context"
	"time"

	"github.com/desek/outlook-local-mcp/internal/resource"
	"github.com/mark3labs/mcp-go/mcp"
)

// NewHandleTrashMessage creates a handler that moves a referenced message to
// Graph's well-known Deleted Items folder without invoking permanent deletion.
func NewHandleTrashMessage(timeout time.Duration, codec *resource.ReferenceCodec) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		target, err := mailTargetFromContext(ctx)
		if err != nil {
			return mcp.NewToolResultError("no account selected"), nil
		}
		sourceID, err := target.referencedMessageID()
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return moveMessageToSemanticDestination(ctx, target, sourceID, "deleteditems", "Message moved to Deleted Items", timeout, codec)
	}
}
