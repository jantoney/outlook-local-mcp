package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/desek/outlook-local-mcp/internal/auth"
	"github.com/desek/outlook-local-mcp/internal/graph"
	"github.com/desek/outlook-local-mcp/internal/resource"
	"github.com/mark3labs/mcp-go/mcp"
)

// HandleListCalendarAliases returns one account's complete calendar alias
// allowlist without making a Graph request.
func HandleListCalendarAliases(registry *auth.AccountRegistry) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
			return mcp.NewToolResultText(formatCalendarAliases(entry.CalendarAliases)), nil
		}
		data, err := json.Marshal(entry.CalendarAliases)
		if err != nil {
			return mcp.NewToolResultError("serialize calendar aliases"), nil
		}
		return mcp.NewToolResultText(string(data)), nil
	}
}

// HandleRenameCalendarAlias returns a local-only handler that changes an
// alias selector while preserving immutable resource and routing identity.
func HandleRenameCalendarAlias(registry *auth.AccountRegistry, accountsPath string) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		accountPolicyMutationMu.Lock()
		defer accountPolicyMutationMu.Unlock()

		entry, alias, index, err := requireCalendarAlias(registry, request)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		newAlias, err := request.RequireString("new_alias")
		if err != nil {
			return mcp.NewToolResultError("missing required parameter: new_alias"), nil
		}
		if _, _, exists := calendarAliasByName(entry.CalendarAliases, newAlias); exists {
			return mcp.NewToolResultError(fmt.Sprintf("calendar alias %q already exists for account %q", newAlias, entry.Label)), nil
		}
		renamed, err := alias.Rename(newAlias)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		aliases := append([]resource.CalendarAlias(nil), entry.CalendarAliases...)
		aliases[index] = renamed
		changed, err := replaceCalendarAliases(registry, accountsPath, entry, aliases)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return calendarAliasMutationResult("renamed", renamed, changed), nil
	}
}

// HandleRemoveCalendarAlias returns an idempotent local-only handler that
// removes one alias identity and thereby invalidates its future resolution.
func HandleRemoveCalendarAlias(registry *auth.AccountRegistry, accountsPath string) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
		entry, ok := registry.Get(label)
		if !ok {
			return mcp.NewToolResultError(fmt.Sprintf("account %q not found", label)), nil
		}
		alias, index, exists := calendarAliasByName(entry.CalendarAliases, name)
		if !exists {
			return mcp.NewToolResultText(fmt.Sprintf("Calendar alias %q is already absent from account %q.", name, label)), nil
		}
		aliases := append([]resource.CalendarAlias(nil), entry.CalendarAliases[:index]...)
		aliases = append(aliases, entry.CalendarAliases[index+1:]...)
		changed, err := replaceCalendarAliases(registry, accountsPath, entry, aliases)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return calendarAliasMutationResult("removed", alias, changed), nil
	}
}

// HandleSetCalendarAliasProfile returns a handler that changes only the exact
// target-local profile and enforces tenant compatibility before persistence.
func HandleSetCalendarAliasProfile(registry *auth.AccountRegistry, accountsPath string) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		accountPolicyMutationMu.Lock()
		defer accountPolicyMutationMu.Unlock()

		entry, alias, index, err := requireCalendarAlias(registry, request)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		profile := resource.CalendarProfile(request.GetString("profile", ""))
		updated, err := alias.WithProfile(profile)
		if err == nil {
			err = resource.ValidateCalendarCompatibility(updated, string(entry.EffectiveTokenTenantContext()))
		}
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		aliases := append([]resource.CalendarAlias(nil), entry.CalendarAliases...)
		aliases[index] = updated
		changed, err := replaceCalendarAliases(registry, accountsPath, entry, aliases)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return calendarAliasMutationResult("updated", updated, changed), nil
	}
}

// HandleReselectCalendarAlias returns a handler that replaces only a mounted
// recipient-view calendar ID after fresh discovery and explicit confirmation.
func HandleReselectCalendarAlias(registry *auth.AccountRegistry, accountsPath string, retryCfg graph.RetryConfig, timeout time.Duration) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		accountPolicyMutationMu.Lock()
		defer accountPolicyMutationMu.Unlock()

		entry, alias, index, err := requireCalendarAlias(registry, request)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if alias.Kind != resource.CalendarKindMounted {
			return mcp.NewToolResultError("only mounted calendar aliases can be reselected"), nil
		}
		selected, err := selectedMountedCalendar(ctx, request, entry, alias.ResourceID, alias.Alias, alias.Owner, alias.Profile, retryCfg, timeout)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		updated, err := alias.ReselectMounted(selected.MountedCalendarID)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		aliases := append([]resource.CalendarAlias(nil), entry.CalendarAliases...)
		aliases[index] = updated
		changed, err := replaceCalendarAliases(registry, accountsPath, entry, aliases)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return calendarAliasMutationResult("reselected", updated, changed), nil
	}
}

// requireCalendarAlias resolves an explicit account and account-scoped alias
// without Graph traffic. It returns the current immutable alias snapshot.
func requireCalendarAlias(registry *auth.AccountRegistry, request mcp.CallToolRequest) (*auth.AccountEntry, resource.CalendarAlias, int, error) {
	label, err := request.RequireString("label")
	if err != nil {
		return nil, resource.CalendarAlias{}, -1, fmt.Errorf("missing required parameter: label")
	}
	name, err := request.RequireString("alias")
	if err != nil {
		return nil, resource.CalendarAlias{}, -1, fmt.Errorf("missing required parameter: alias")
	}
	entry, ok := registry.Get(label)
	if !ok {
		return nil, resource.CalendarAlias{}, -1, fmt.Errorf("account %q not found", label)
	}
	alias, index, ok := calendarAliasByName(entry.CalendarAliases, name)
	if !ok {
		return nil, resource.CalendarAlias{}, -1, fmt.Errorf("calendar alias %q not found for account %q", name, label)
	}
	return entry, alias, index, nil
}

// calendarAliasMutationResult formats a write confirmation and reports whether
// the effective OAuth scope union forced disconnection.
func calendarAliasMutationResult(action string, alias resource.CalendarAlias, scopesChanged bool) *mcp.CallToolResult {
	message := fmt.Sprintf("Calendar alias %q %s with immutable resource ID %s (%s, %s, profile=%s).", alias.Alias, action, alias.ResourceID, alias.Kind, alias.View, alias.Profile)
	if scopesChanged {
		message += " OAuth scopes changed; local authentication was cleared. Call account.login to reconnect."
	} else {
		message += " OAuth scopes are unchanged."
	}
	return mcp.NewToolResultText(message)
}

// formatCalendarAliases formats the complete local alias records as a compact
// numbered text list.
func formatCalendarAliases(aliases []resource.CalendarAlias) string {
	if len(aliases) == 0 {
		return "No calendar aliases configured."
	}
	lines := make([]string, 0, len(aliases))
	for index, alias := range aliases {
		lines = append(lines, fmt.Sprintf("%d. %s — %s (%s, %s, profile=%s, resource_id=%s)", index+1, alias.Alias, alias.Owner, alias.Kind, alias.View, alias.Profile, alias.ResourceID))
	}
	return strings.Join(lines, "\n")
}
