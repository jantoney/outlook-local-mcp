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

// HandleDiscoverCalendarAliases returns a read handler that discovers mounted
// recipient-view calendars for one explicit account and owner. Discovery never
// persists or silently selects a candidate.
func HandleDiscoverCalendarAliases(registry *auth.AccountRegistry, retryCfg graph.RetryConfig, timeout time.Duration) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		label, err := request.RequireString("label")
		if err != nil {
			return mcp.NewToolResultError("missing required parameter: label"), nil
		}
		owner, err := request.RequireString("owner")
		if err != nil {
			return mcp.NewToolResultError("missing required parameter: owner"), nil
		}
		entry, ok := registry.Get(label)
		if !ok {
			return mcp.NewToolResultError(fmt.Sprintf("account %q not found", label)), nil
		}
		candidates, err := discoverMountedCalendarCandidates(ctx, entry, owner, retryCfg, timeout)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		mode, err := ValidateOutputMode(request)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if mode == "text" {
			if len(candidates) == 0 {
				return mcp.NewToolResultText("No mounted calendars found for the configured owner."), nil
			}
			lines := make([]string, 0, len(candidates))
			for index, candidate := range candidates {
				lines = append(lines, fmt.Sprintf("%d. %s — ID: %s (owner=%s, can_edit=%t)", index+1, candidate.Name, candidate.ID, candidate.Owner, candidate.CanEdit))
			}
			return mcp.NewToolResultText(strings.Join(lines, "\n")), nil
		}
		data, err := json.Marshal(candidates)
		if err != nil {
			return mcp.NewToolResultError("serialize mounted calendar candidates"), nil
		}
		return mcp.NewToolResultText(string(data)), nil
	}
}

// HandleAddCalendarAlias returns a handler that creates one immutable
// account-scoped owner-primary or explicitly selected mounted calendar alias.
func HandleAddCalendarAlias(registry *auth.AccountRegistry, accountsPath string, retryCfg graph.RetryConfig, timeout time.Duration) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		label, aliasName, owner, kind, profile, entry, err := calendarAliasCreateArguments(registry, request)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if _, _, exists := calendarAliasByName(entry.CalendarAliases, aliasName); exists {
			return mcp.NewToolResultError(fmt.Sprintf("calendar alias %q already exists for account %q", aliasName, label)), nil
		}
		id, err := resource.NewResourceID()
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		var calendar resource.CalendarAlias
		switch kind {
		case resource.CalendarKindOwnerPrimary:
			calendar, err = resource.NewOwnerPrimaryCalendar(id, aliasName, owner, profile)
		case resource.CalendarKindMounted:
			calendar, err = selectedMountedCalendar(ctx, request, entry, id, aliasName, owner, profile, retryCfg, timeout)
		default:
			err = fmt.Errorf("invalid calendar kind %q", kind)
		}
		if err == nil {
			err = resource.ValidateCalendarCompatibility(calendar, string(entry.EffectiveTokenTenantContext()))
		}
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		aliases := append(append([]resource.CalendarAlias(nil), entry.CalendarAliases...), calendar)
		changed, err := replaceCalendarAliases(registry, accountsPath, entry, aliases)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return calendarAliasMutationResult("added", calendar, changed), nil
	}
}

// calendarAliasCreateArguments parses add arguments and resolves the explicit
// signed-in account without performing Graph traffic.
func calendarAliasCreateArguments(registry *auth.AccountRegistry, request mcp.CallToolRequest) (string, string, string, resource.CalendarKind, resource.CalendarProfile, *auth.AccountEntry, error) {
	label, err := request.RequireString("label")
	if err != nil {
		return "", "", "", "", "", nil, fmt.Errorf("missing required parameter: label")
	}
	aliasName, err := request.RequireString("alias")
	if err != nil {
		return "", "", "", "", "", nil, fmt.Errorf("missing required parameter: alias")
	}
	owner, err := request.RequireString("owner")
	if err != nil {
		return "", "", "", "", "", nil, fmt.Errorf("missing required parameter: owner")
	}
	kind := resource.CalendarKind(request.GetString("kind", ""))
	profile := resource.CalendarProfile(request.GetString("profile", string(resource.CalendarProfileOff)))
	entry, ok := registry.Get(label)
	if !ok {
		return "", "", "", "", "", nil, fmt.Errorf("account %q not found", label)
	}
	return label, aliasName, owner, kind, profile, entry, nil
}

// selectedMountedCalendar validates explicit human confirmation against fresh
// /me/calendars discovery and never chooses a candidate automatically.
func selectedMountedCalendar(ctx context.Context, request mcp.CallToolRequest, entry *auth.AccountEntry, id resource.ResourceID, aliasName, owner string, profile resource.CalendarProfile, retryCfg graph.RetryConfig, timeout time.Duration) (resource.CalendarAlias, error) {
	candidates, err := discoverMountedCalendarCandidates(ctx, entry, owner, retryCfg, timeout)
	if err != nil {
		return resource.CalendarAlias{}, err
	}
	selectedID := request.GetString("mounted_calendar_id", "")
	confirmed, _ := request.GetArguments()["confirm_mounted_selection"].(bool)
	if selectedID == "" || !confirmed {
		data, _ := json.Marshal(candidates)
		return resource.CalendarAlias{}, fmt.Errorf("explicit mounted-calendar selection required; choose one candidate ID and repeat with confirm_mounted_selection=true: %s", data)
	}
	for _, candidate := range candidates {
		if candidate.ID != selectedID {
			continue
		}
		if profile == resource.CalendarProfileManage && !candidate.CanEdit {
			return resource.CalendarAlias{}, fmt.Errorf("selected mounted calendar is not editable")
		}
		return resource.NewMountedCalendar(id, aliasName, owner, selectedID, profile)
	}
	return resource.CalendarAlias{}, fmt.Errorf("selected mounted calendar %q was not returned by fresh discovery for owner %q", selectedID, owner)
}
