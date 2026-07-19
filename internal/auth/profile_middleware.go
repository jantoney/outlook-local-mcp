package auth

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

// RequireMailProfile protects a mail handler with the selected account's
// minimum capability profile. It rejects before any Graph call when the
// account context is missing or insufficient.
func RequireMailProfile(required MailProfile, next mcpserver.ToolHandlerFunc) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		info, ok := AccountInfoFromContext(ctx)
		if !ok {
			return mcp.NewToolResultError("no account capability profile selected"), nil
		}
		if !info.MailProfile.Allows(required) {
			return mcp.NewToolResultError(fmt.Sprintf(
				"account %q has mail profile %s; this operation requires %s",
				info.Label, info.MailProfile, required,
			)), nil
		}
		return next(ctx, request)
	}
}
