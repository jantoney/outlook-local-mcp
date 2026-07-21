package tools

import (
	"context"
	"time"

	"github.com/desek/outlook-local-mcp/internal/resource"
	"github.com/mark3labs/mcp-go/mcp"
)

// NewHandleArchiveMessage creates a handler for Outlook One-Click Archive. It
// always uses Graph's well-known archive destination and does not address the
// separate Exchange Online Archive Mailbox.
func NewHandleArchiveMessage(timeout time.Duration, codec *resource.ReferenceCodec) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		target, err := mailTargetFromContext(ctx)
		if err != nil {
			return mcp.NewToolResultError("no account selected"), nil
		}
		sourceID, err := target.referencedMessageID()
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return moveMessageToSemanticDestination(ctx, target, sourceID, "archive", "Message archived to Outlook One-Click Archive", timeout, codec)
	}
}
