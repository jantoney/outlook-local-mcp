package auth

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

// RequireMailCapability protects a mail handler with one exact own-mail
// capability. It fails closed before the handler can construct a Graph route
// when account context is absent or the selected action is disabled.
//
// Parameters:
//   - required: exact action the protected handler performs.
//   - next: handler invoked only when the policy allows required.
//
// Returns middleware-compatible handler output. It has no side effects when
// authorization fails and propagates the wrapped handler's result otherwise.
func RequireMailCapability(required MailCapability, next mcpserver.ToolHandlerFunc) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		info, ok := AccountInfoFromContext(ctx)
		if !ok {
			return mcp.NewToolResultError("no account mail action policy selected"), nil
		}
		if !info.MailPolicy.Allows(required) {
			return mcp.NewToolResultError(fmt.Sprintf(
				"account %q own-mail policy disables %s",
				info.Label, required,
			)), nil
		}
		return next(ctx, request)
	}
}
