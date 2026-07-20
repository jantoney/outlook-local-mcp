package tools

import (
	"context"

	"github.com/desek/outlook-local-mcp/internal/auth"
	"github.com/mark3labs/mcp-go/mcp"
)

// HandleSetMailProfile returns the legacy transition handler for cumulative
// profile input. It deterministically maps the requested profile to a complete
// independent policy and delegates persistence and reauthentication decisions
// to HandleSetMailPolicy.
func HandleSetMailProfile(registry *auth.AccountRegistry, accountsPath string) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	setPolicy := HandleSetMailPolicy(registry, accountsPath)
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		label, err := request.RequireString("label")
		if err != nil {
			return mcp.NewToolResultError("missing required parameter: label"), nil
		}
		profileName, err := request.RequireString("mail_profile")
		if err != nil {
			return mcp.NewToolResultError("missing required parameter: mail_profile"), nil
		}
		profile, err := auth.ParseMailProfile(profileName)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		policy := auth.MailPolicyFromProfile(profile)
		request.Params.Arguments = map[string]any{
			"label": label, "read": policy.Read, "draft": policy.Draft,
			"move": policy.Move, "archive": policy.Archive, "trash": policy.Trash,
			"restore": policy.Restore, "permanent_delete": policy.PermanentDelete,
			"send": policy.Send,
		}
		return setPolicy(ctx, request)
	}
}
