package tools

import (
	"context"
	"encoding/json"

	"github.com/desek/outlook-local-mcp/internal/accountadmin"
	"github.com/mark3labs/mcp-go/mcp"
)

// HandleRefreshSharedResources returns an MCP adapter for observational
// calendar discovery and direct shared-resource metadata validation.
func HandleRefreshSharedResources(admin *accountadmin.Module) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		label, err := request.RequireString("label")
		if err != nil {
			return mcp.NewToolResultError("missing required parameter: label"), nil
		}
		result, err := admin.RefreshSharedResources(ctx, label)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		data, err := json.Marshal(result)
		if err != nil {
			return mcp.NewToolResultError("serialize shared-resource refresh"), nil
		}
		return mcp.NewToolResultText(string(data)), nil
	}
}
