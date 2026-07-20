package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/desek/outlook-local-mcp/internal/auth"
	"github.com/desek/outlook-local-mcp/internal/resource"
	"github.com/mark3labs/mcp-go/mcp"
)

// HandleListMailAliases returns one account's complete shared-mail allowlist,
// including exact independent action policies, without Graph traffic.
func HandleListMailAliases(registry *auth.AccountRegistry) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		label, err := request.RequireString("label")
		if err != nil {
			return mcp.NewToolResultError("missing required parameter: label"), nil
		}
		entry, ok := registry.Get(label)
		if !ok {
			return mcp.NewToolResultError(fmt.Sprintf("account %q not found", label)), nil
		}
		mode, err := ValidateOutputMode(request)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if mode == "text" {
			return mcp.NewToolResultText(formatMailAliases(entry.MailAliases, entry.EffectiveTokenTenantContext())), nil
		}
		data, err := json.Marshal(entry.MailAliases)
		if err != nil {
			return mcp.NewToolResultError("serialize mail aliases"), nil
		}
		return mcp.NewToolResultText(string(data)), nil
	}
}

// HandleRenameMailAlias changes only the human selector for an organizational
// target and preserves immutable identity, owner, kind, view, and policy.
func HandleRenameMailAlias(registry *auth.AccountRegistry, accountsPath string) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		entry, alias, index, err := requireMailAlias(registry, request)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		recordMailAliasAuditTarget(ctx, alias, mailAliasCompatibility(entry))
		if err := resource.ValidateMailCompatibility(alias, string(entry.EffectiveTokenTenantContext())); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		newName, err := request.RequireString("new_alias")
		if err != nil {
			return mcp.NewToolResultError("missing required parameter: new_alias"), nil
		}
		if _, _, exists := mailAliasByName(entry.MailAliases, newName); exists {
			return mcp.NewToolResultError(fmt.Sprintf("mail alias %q already exists for account %q", newName, entry.Label)), nil
		}
		renamed, err := alias.Rename(newName)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		recordMailAliasAuditTarget(ctx, renamed, "compatible")
		aliases := append([]resource.MailAlias(nil), entry.MailAliases...)
		aliases[index] = renamed
		changed, err := replaceMailAliases(registry, accountsPath, entry, aliases)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mailAliasMutationResult("renamed", renamed, changed), nil
	}
}

// HandleRemoveMailAlias idempotently removes one local alias identity. Removal
// remains available in incompatible contexts so invalid configuration can be
// recovered without Graph traffic.
func HandleRemoveMailAlias(registry *auth.AccountRegistry, accountsPath string) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		label, err := request.RequireString("label")
		if err != nil {
			return mcp.NewToolResultError("missing required parameter: label"), nil
		}
		name, err := request.RequireString("alias")
		if err != nil {
			return mcp.NewToolResultError("missing required parameter: alias"), nil
		}
		entry, ok := registry.Get(label)
		if !ok {
			return mcp.NewToolResultError(fmt.Sprintf("account %q not found", label)), nil
		}
		alias, index, exists := mailAliasByName(entry.MailAliases, name)
		if !exists {
			return mcp.NewToolResultText(fmt.Sprintf("Mail alias %q is already absent from account %q.", name, label)), nil
		}
		recordMailAliasAuditTarget(ctx, alias, mailAliasCompatibility(entry))
		aliases := append([]resource.MailAlias(nil), entry.MailAliases[:index]...)
		aliases = append(aliases, entry.MailAliases[index+1:]...)
		changed, err := replaceMailAliases(registry, accountsPath, entry, aliases)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mailAliasMutationResult("removed", alias, changed), nil
	}
}

// HandleSetMailAliasPolicy partially updates the exact target-local action
// matrix. Omitted switches remain unchanged and compatibility is checked before
// persistence or any later Graph routing.
func HandleSetMailAliasPolicy(registry *auth.AccountRegistry, accountsPath string) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		entry, alias, index, err := requireMailAlias(registry, request)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		recordMailAliasAuditTarget(ctx, alias, mailAliasCompatibility(entry))
		if err := resource.ValidateMailCompatibility(alias, string(entry.EffectiveTokenTenantContext())); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		policy, changedField, err := mailPolicyFromArguments(alias.Policy, request.GetArguments())
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if !changedField {
			return mcp.NewToolResultError("at least one mail action switch is required"), nil
		}
		updated := alias.WithPolicy(policy)
		recordMailAliasAuditTarget(ctx, updated, "compatible")
		aliases := append([]resource.MailAlias(nil), entry.MailAliases...)
		aliases[index] = updated
		changedScopes, err := replaceMailAliases(registry, accountsPath, entry, aliases)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mailAliasMutationResult("updated", updated, changedScopes), nil
	}
}

// recordMailAliasAuditTarget publishes the exact selected target snapshot to
// outer audit middleware without re-reading mutable registry state.
func recordMailAliasAuditTarget(ctx context.Context, alias resource.MailAlias, compatibility string) {
	auth.RecordAuditTarget(ctx, auth.AuditTarget{
		Alias:         alias.Alias,
		ResourceID:    alias.ResourceID,
		ResourceKind:  alias.Kind,
		MailboxView:   alias.View,
		MailPolicy:    alias.Policy,
		Compatibility: compatibility,
	})
}

// mailAliasCompatibility returns the explicit local compatibility label for
// one account's authoritative token tenant context.
func mailAliasCompatibility(entry *auth.AccountEntry) string {
	if entry != nil && entry.EffectiveTokenTenantContext() == auth.TokenTenantOrganizational {
		return "compatible"
	}
	return "incompatible"
}

// requireMailAlias resolves an explicit account and account-scoped alias from
// local state. It never constructs a Graph route or makes network traffic.
func requireMailAlias(registry *auth.AccountRegistry, request mcp.CallToolRequest) (*auth.AccountEntry, resource.MailAlias, int, error) {
	label, err := request.RequireString("label")
	if err != nil {
		return nil, resource.MailAlias{}, -1, fmt.Errorf("missing required parameter: label")
	}
	name, err := request.RequireString("alias")
	if err != nil {
		return nil, resource.MailAlias{}, -1, fmt.Errorf("missing required parameter: alias")
	}
	entry, ok := registry.Get(label)
	if !ok {
		return nil, resource.MailAlias{}, -1, fmt.Errorf("account %q not found", label)
	}
	alias, index, ok := mailAliasByName(entry.MailAliases, name)
	if !ok {
		return nil, resource.MailAlias{}, -1, fmt.Errorf("mail alias %q not found for account %q", name, label)
	}
	return entry, alias, index, nil
}

// mailAliasMutationResult formats a write confirmation with immutable target
// identity, exact policy, compatibility, and scope-change behavior.
func mailAliasMutationResult(action string, alias resource.MailAlias, scopesChanged bool) *mcp.CallToolResult {
	message := fmt.Sprintf("Mail alias %q %s with immutable resource ID %s (%s, %s, policy=%s, compatibility=organizational-only).", alias.Alias, action, alias.ResourceID, alias.Kind, alias.View, formatMailPolicyText(alias.Policy))
	if scopesChanged {
		message += " OAuth scopes changed; local authentication was cleared. Call account.login to reconnect."
	} else {
		message += " OAuth scopes are unchanged."
	}
	return mcp.NewToolResultText(message)
}

// formatMailAliases formats exact shared-mail identities and policies as a
// compact numbered list with the selected account's current compatibility.
func formatMailAliases(aliases []resource.MailAlias, tenantContext auth.TokenTenantContext) string {
	if len(aliases) == 0 {
		return "No mail aliases configured."
	}
	compatibility := "incompatible"
	if tenantContext == auth.TokenTenantOrganizational {
		compatibility = "compatible"
	}
	lines := make([]string, 0, len(aliases))
	for index, alias := range aliases {
		lines = append(lines, fmt.Sprintf("%d. %s — %s (%s, %s, policy=%s, compatibility=%s, resource_id=%s)", index+1, alias.Alias, alias.Owner, alias.Kind, alias.View, formatMailPolicyText(alias.Policy), compatibility, alias.ResourceID))
	}
	return strings.Join(lines, "\n")
}
