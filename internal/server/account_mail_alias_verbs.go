package server

import (
	"github.com/desek/outlook-local-mcp/internal/tools"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

// buildMailAliasVerbs constructs the account-domain lifecycle surface for
// organizational owner-view shared mailboxes. Every mutation is read-only
// guarded before local persistence or any future Graph routing.
func buildMailAliasVerbs(c accountVerbsConfig, wrap func(string, string, mcpserver.ToolHandlerFunc) tools.Handler) []tools.Verb {
	identitySchema := []mcp.ToolOption{
		mcp.WithString("label", mcp.Required(), mcp.Description("Signed-in account label that owns this alias namespace.")),
		mcp.WithString("alias", mcp.Required(), mcp.Description("Account-scoped shared-mail alias.")),
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
			Name:        "add_mail_alias",
			Summary:     "add an organizational shared mailbox with every action disabled",
			Description: "Adds one account-scoped owner-view mailbox identity. The exact eight-action policy defaults entirely off. Personal and unknown token contexts are rejected locally; Exchange delegation remains authoritative.",
			SeeDocs:     []string{"concepts#shared-mail-aliases", "concepts#token-tenant-context-and-oauth-scope-union"},
			Handler: wrap("account.add_mail_alias", "write", ReadOnlyGuard(
				"account.add_mail_alias", c.cfg.ReadOnly, tools.HandleAddMailAlias(c.registry, c.cfg.AccountsPath))),
			Annotations: []mcp.ToolOption{
				mcp.WithReadOnlyHintAnnotation(false),
				mcp.WithDestructiveHintAnnotation(false),
				mcp.WithIdempotentHintAnnotation(false),
				mcp.WithOpenWorldHintAnnotation(false),
			},
			Schema: []mcp.ToolOption{
				mcp.WithString("label", mcp.Required(), mcp.Description("Organizational signed-in account label.")),
				mcp.WithString("alias", mcp.Required(), mcp.Description("New account-scoped mail selector.")),
				mcp.WithString("owner", mcp.Required(), mcp.Description("Immutable Graph-compatible mailbox owner locator.")),
			},
		},
		{
			Name:        "list_mail_aliases",
			Summary:     "list shared-mail identities, exact policies, and compatibility",
			Description: "Lists immutable owner-view identity, every independent target action, and organizational compatibility without Graph traffic.",
			SeeDocs:     []string{"concepts#shared-mail-aliases"},
			Handler:     wrap("account.list_mail_aliases", "read", tools.HandleListMailAliases(c.registry)),
			Annotations: readAnnotations,
			Schema: []mcp.ToolOption{
				mcp.WithString("label", mcp.Required(), mcp.Description("Signed-in account label.")),
				mcp.WithString("output", mcp.Description("Output mode: text (default), summary, or raw."), mcp.Enum("text", "summary", "raw")),
			},
		},
		{
			Name:        "rename_mail_alias",
			Summary:     "rename a mail selector without changing mailbox identity",
			Description: "Changes only the human-facing selector. Immutable resource ID, owner, mailbox kind, owner view, and exact target policy remain unchanged.",
			SeeDocs:     []string{"concepts#shared-mail-aliases"},
			Handler: wrap("account.rename_mail_alias", "write", ReadOnlyGuard(
				"account.rename_mail_alias", c.cfg.ReadOnly, tools.HandleRenameMailAlias(c.registry, c.cfg.AccountsPath))),
			Annotations: writeAnnotations,
			Schema: append(identitySchema,
				mcp.WithString("new_alias", mcp.Required(), mcp.Description("New account-scoped selector."))),
		},
		{
			Name:        "remove_mail_alias",
			Summary:     "idempotently remove and revoke one shared-mail identity",
			Description: "Removes the account-scoped alias. Recreating the same selector creates a new immutable identity, so references bound to the removed target cannot revive.",
			SeeDocs:     []string{"concepts#shared-mail-aliases"},
			Handler: wrap("account.remove_mail_alias", "write", ReadOnlyGuard(
				"account.remove_mail_alias", c.cfg.ReadOnly, tools.HandleRemoveMailAlias(c.registry, c.cfg.AccountsPath))),
			Annotations: []mcp.ToolOption{
				mcp.WithReadOnlyHintAnnotation(false),
				mcp.WithDestructiveHintAnnotation(true),
				mcp.WithIdempotentHintAnnotation(true),
				mcp.WithOpenWorldHintAnnotation(false),
			},
			Schema: identitySchema,
		},
		{
			Name:        "set_mail_alias_policy",
			Summary:     "change exact independent actions for one shared mailbox",
			Description: "Partially updates only the selected alias policy; omitted switches remain unchanged and own-mail rights do not constrain it. Organizational compatibility is checked locally. The account disconnects only if the complete OAuth scope union changes.",
			SeeDocs:     []string{"concepts#shared-mail-aliases", "concepts#independent-mail-action-policies", "concepts#token-tenant-context-and-oauth-scope-union"},
			Handler: wrap("account.set_mail_alias_policy", "write", ReadOnlyGuard(
				"account.set_mail_alias_policy", c.cfg.ReadOnly, tools.HandleSetMailAliasPolicy(c.registry, c.cfg.AccountsPath))),
			Annotations: writeAnnotations,
			Schema:      append(identitySchema, mailPolicySchema()...),
		},
	}
}
