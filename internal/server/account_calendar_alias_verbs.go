package server

import (
	"time"

	"github.com/desek/outlook-local-mcp/internal/graph"
	"github.com/desek/outlook-local-mcp/internal/tools"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

// buildCalendarAliasVerbs constructs the account-domain discovery and
// lifecycle surface for account-scoped shared calendar aliases. Mutation
// handlers are read-only guarded before local validation or Graph discovery.
func buildCalendarAliasVerbs(c accountVerbsConfig, wrap func(string, string, mcpserver.ToolHandlerFunc) tools.Handler) []tools.Verb {
	retryCfg := graph.RetryConfig{
		MaxRetries:     c.cfg.MaxRetries,
		InitialBackoff: time.Duration(c.cfg.RetryBackoffMS) * time.Millisecond,
	}
	commonIdentity := []mcp.ToolOption{
		mcp.WithString("label", mcp.Required(), mcp.Description("Signed-in account label that owns this alias namespace.")),
		mcp.WithString("alias", mcp.Required(), mcp.Description("Account-scoped calendar alias.")),
	}
	mountedSelection := []mcp.ToolOption{
		mcp.WithString("mounted_calendar_id", mcp.Description("Exact recipient-view calendar ID returned by fresh discovery.")),
		mcp.WithBoolean("confirm_mounted_selection", mcp.Description("Must be true to confirm the human selected mounted_calendar_id from discovery.")),
	}
	readAnnotations := []mcp.ToolOption{
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
	}
	writeAnnotations := []mcp.ToolOption{
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
	}

	return []tools.Verb{
		{
			Name:        "discover_calendar_aliases",
			Summary:     "discover mounted recipient-view calendars for an owner",
			Description: "Calls /me/calendars for one connected account and returns only calendars whose Graph owner address matches the configured owner. Discovery never persists or selects a calendar.",
			SeeDocs:     []string{"concepts#shared-calendar-aliases"},
			Handler:     wrap("account.discover_calendar_aliases", "read", tools.HandleDiscoverCalendarAliases(c.registry, retryCfg, c.cfg.RequestTimeout)),
			Annotations: []mcp.ToolOption{
				mcp.WithReadOnlyHintAnnotation(true),
				mcp.WithDestructiveHintAnnotation(false),
				mcp.WithIdempotentHintAnnotation(true),
				mcp.WithOpenWorldHintAnnotation(true),
			},
			Schema: []mcp.ToolOption{
				mcp.WithString("label", mcp.Required(), mcp.Description("Connected signed-in account label.")),
				mcp.WithString("owner", mcp.Required(), mcp.Description("Exact owner locator used to filter Graph calendar ownership.")),
				mcp.WithString("output", mcp.Description("Output mode: text (default), summary, or raw."), mcp.Enum("text", "summary", "raw")),
			},
		},
		{
			Name:        "add_calendar_alias",
			Summary:     "add an immutable owner-primary or selected mounted calendar alias",
			Description: "Adds one account-scoped calendar alias. Mounted creation always performs fresh /me/calendars discovery and requires the exact returned ID plus confirm_mounted_selection=true. Owner-primary aliases are organizational and read-only; mounted manage requires organizational context.",
			SeeDocs:     []string{"concepts#shared-calendar-aliases", "concepts#token-tenant-context-and-oauth-scope-union"},
			Handler: wrap("account.add_calendar_alias", "write", ReadOnlyGuard(
				"account.add_calendar_alias", c.cfg.ReadOnly,
				tools.HandleAddCalendarAlias(c.registry, c.cfg.AccountsPath, retryCfg, c.cfg.RequestTimeout))),
			Annotations: []mcp.ToolOption{
				mcp.WithReadOnlyHintAnnotation(false),
				mcp.WithDestructiveHintAnnotation(false),
				mcp.WithIdempotentHintAnnotation(false),
				mcp.WithOpenWorldHintAnnotation(true),
			},
			Schema: append([]mcp.ToolOption{
				mcp.WithString("label", mcp.Required(), mcp.Description("Signed-in account label.")),
				mcp.WithString("alias", mcp.Required(), mcp.Description("New account-scoped alias.")),
				mcp.WithString("owner", mcp.Required(), mcp.Description("Immutable Graph-compatible resource owner locator.")),
				mcp.WithString("kind", mcp.Required(), mcp.Description("Immutable mailbox-view kind."), mcp.Enum("owner_primary_calendar", "mounted_calendar")),
				mcp.WithString("profile", mcp.Required(), mcp.Description("Target-local capability profile."), mcp.Enum("off", "read", "manage")),
			}, mountedSelection...),
		},
		{
			Name:        "list_calendar_aliases",
			Summary:     "list one account's complete shared-calendar allowlist",
			Description: "Lists immutable resource identity, alias, owner, kind, mailbox view, mounted ID, and exact target-local profile without Graph traffic.",
			SeeDocs:     []string{"concepts#shared-calendar-aliases"},
			Handler:     wrap("account.list_calendar_aliases", "read", tools.HandleListCalendarAliases(c.registry)),
			Annotations: readAnnotations,
			Schema: []mcp.ToolOption{
				mcp.WithString("label", mcp.Required(), mcp.Description("Signed-in account label.")),
				mcp.WithString("output", mcp.Description("Output mode: text (default), summary, or raw."), mcp.Enum("text", "summary", "raw")),
			},
		},
		{
			Name:        "rename_calendar_alias",
			Summary:     "rename a calendar selector without changing resource identity",
			Description: "Changes only the human-facing alias. Owner, kind, view, mounted ID, and immutable resource identity remain unchanged.",
			SeeDocs:     []string{"concepts#shared-calendar-aliases"},
			Handler: wrap("account.rename_calendar_alias", "write", ReadOnlyGuard(
				"account.rename_calendar_alias", c.cfg.ReadOnly,
				tools.HandleRenameCalendarAlias(c.registry, c.cfg.AccountsPath))),
			Annotations: writeAnnotations,
			Schema: append(commonIdentity,
				mcp.WithString("new_alias", mcp.Required(), mcp.Description("New account-scoped selector."))),
		},
		{
			Name:        "remove_calendar_alias",
			Summary:     "idempotently remove and revoke one calendar alias identity",
			Description: "Removes the account-scoped alias. Recreating the same name produces a new immutable identity; old item references do not revive.",
			SeeDocs:     []string{"concepts#shared-calendar-aliases"},
			Handler: wrap("account.remove_calendar_alias", "write", ReadOnlyGuard(
				"account.remove_calendar_alias", c.cfg.ReadOnly,
				tools.HandleRemoveCalendarAlias(c.registry, c.cfg.AccountsPath))),
			Annotations: []mcp.ToolOption{
				mcp.WithReadOnlyHintAnnotation(false),
				mcp.WithDestructiveHintAnnotation(true),
				mcp.WithIdempotentHintAnnotation(true),
				mcp.WithOpenWorldHintAnnotation(false),
			},
			Schema: commonIdentity,
		},
		{
			Name:        "set_calendar_alias_profile",
			Summary:     "set the exact off, read, or manage profile for one calendar alias",
			Description: "Changes only target-local authorization. Owner-primary manage and non-organizational manage are rejected locally. Authentication is cleared only when the account-wide OAuth scope union changes.",
			SeeDocs:     []string{"concepts#shared-calendar-aliases", "concepts#token-tenant-context-and-oauth-scope-union"},
			Handler: wrap("account.set_calendar_alias_profile", "write", ReadOnlyGuard(
				"account.set_calendar_alias_profile", c.cfg.ReadOnly,
				tools.HandleSetCalendarAliasProfile(c.registry, c.cfg.AccountsPath))),
			Annotations: writeAnnotations,
			Schema: append(commonIdentity,
				mcp.WithString("profile", mcp.Required(), mcp.Description("Target-local profile."), mcp.Enum("off", "read", "manage"))),
		},
		{
			Name:        "reselect_calendar_alias",
			Summary:     "reselect a missing mounted calendar without rebinding silently",
			Description: "Performs fresh owner-filtered discovery and replaces the mounted calendar ID after explicit human confirmation. A fresh immutable resource identity invalidates references from the prior selection; alias, owner, kind, view, and policy are preserved.",
			SeeDocs:     []string{"concepts#shared-calendar-aliases"},
			Handler: wrap("account.reselect_calendar_alias", "write", ReadOnlyGuard(
				"account.reselect_calendar_alias", c.cfg.ReadOnly,
				tools.HandleReselectCalendarAlias(c.registry, c.cfg.AccountsPath, retryCfg, c.cfg.RequestTimeout))),
			Annotations: []mcp.ToolOption{
				mcp.WithReadOnlyHintAnnotation(false),
				mcp.WithDestructiveHintAnnotation(false),
				mcp.WithIdempotentHintAnnotation(false),
				mcp.WithOpenWorldHintAnnotation(true),
			},
			Schema: append(commonIdentity, mountedSelection...),
		},
	}
}
