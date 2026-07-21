package tools

import (
	"context"
	"fmt"

	"github.com/desek/outlook-local-mcp/internal/auth"
	"github.com/desek/outlook-local-mcp/internal/resource"
	"github.com/mark3labs/mcp-go/mcp"
)

// HandleAddMailAlias creates one organizational owner-view mailbox alias with
// a new immutable identity and every action disabled by default. Compatibility
// is checked locally before persistence and no Graph request is made.
func HandleAddMailAlias(registry *auth.AccountRegistry, accountsPath string) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		accountPolicyMutationMu.Lock()
		defer accountPolicyMutationMu.Unlock()

		label, err := request.RequireString("label")
		if err != nil {
			return mcp.NewToolResultError("missing required parameter: label"), nil
		}
		name, err := request.RequireString("alias")
		if err != nil {
			return mcp.NewToolResultError("missing required parameter: alias"), nil
		}
		owner, err := request.RequireString("owner")
		if err != nil {
			return mcp.NewToolResultError("missing required parameter: owner"), nil
		}
		entry, ok := registry.Get(label)
		if !ok {
			return mcp.NewToolResultError(fmt.Sprintf("account %q not found", label)), nil
		}
		if _, _, exists := mailAliasByName(entry.MailAliases, name); exists {
			return mcp.NewToolResultError(fmt.Sprintf("mail alias %q already exists for account %q", name, label)), nil
		}
		id, err := resource.NewResourceID()
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		alias, err := resource.NewMailAlias(id, name, owner)
		if err == nil {
			recordMailAliasAuditTarget(ctx, alias, mailAliasCompatibility(entry))
			err = resource.ValidateMailCompatibility(alias, string(entry.EffectiveTokenTenantContext()))
		}
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		aliases := append(append([]resource.MailAlias(nil), entry.MailAliases...), alias)
		changed, err := replaceMailAliases(registry, accountsPath, entry, aliases)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mailAliasMutationResult("added", alias, changed), nil
	}
}
