package server

import (
	"github.com/desek/outlook-local-mcp/internal/graph"
	"github.com/desek/outlook-local-mcp/internal/tools"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

// buildArchiveMessageVerb constructs the separately gated Outlook One-Click
// Archive verb. It never targets the Exchange Online Archive Mailbox.
func buildArchiveMessageVerb(c mailVerbsConfig, wrapWrite func(string, string, mcpserver.ToolHandlerFunc) tools.Handler) tools.Verb {
	return tools.Verb{
		Name: "archive_message", Summary: "archive one referenced message",
		Description: "Moves one referenced own or shared-mail message to Outlook's well-known One-Click Archive folder. This does not target the separate Exchange Online Archive Mailbox. Requires the exact archive capability and returns the moved message's new reference.",
		SeeDocs:     []string{"concepts#mail-filing-and-recovery"},
		Handler:     wrapWrite("mail.archive_message", "write", tools.NewHandleArchiveMessage(c.timeout, c.referenceCodec)),
		Annotations: []mcp.ToolOption{
			mcp.WithReadOnlyHintAnnotation(false), mcp.WithDestructiveHintAnnotation(false),
			mcp.WithIdempotentHintAnnotation(false), mcp.WithOpenWorldHintAnnotation(true),
		},
		Schema: referencedMessageMutationSchema(),
	}
}

// buildTrashMessageVerb constructs the separately gated reversible Deleted
// Items move and does not expose Graph permanent deletion.
func buildTrashMessageVerb(c mailVerbsConfig, wrapWrite func(string, string, mcpserver.ToolHandlerFunc) tools.Handler) tools.Verb {
	return tools.Verb{
		Name: "trash_message", Summary: "move one referenced message to Deleted Items",
		Description: "Moves one referenced own or shared-mail message to Graph's well-known Deleted Items folder. This is distinct from permanent deletion. Requires the exact trash capability and returns the message's new Deleted Items reference.",
		SeeDocs:     []string{"concepts#mail-filing-and-recovery"},
		Handler:     wrapWrite("mail.trash_message", "write", tools.NewHandleTrashMessage(c.timeout, c.referenceCodec)),
		Annotations: []mcp.ToolOption{
			mcp.WithReadOnlyHintAnnotation(false), mcp.WithDestructiveHintAnnotation(false),
			mcp.WithIdempotentHintAnnotation(false), mcp.WithOpenWorldHintAnnotation(true),
		},
		Schema: referencedMessageMutationSchema(),
	}
}

// buildRestoreMessageVerb constructs the independently gated Deleted Items to
// ordinary-folder restoration verb.
func buildRestoreMessageVerb(c mailVerbsConfig, retryCfg graph.RetryConfig, wrapWrite func(string, string, mcpserver.ToolHandlerFunc) tools.Handler) tools.Verb {
	return tools.Verb{
		Name: "restore_message", Summary: "restore a Deleted Items message to an ordinary folder",
		Description: "Verifies that one referenced message is currently in Deleted Items, then moves it to a same-target folder reference classified as ordinary. Archive, deletion-class, reserved, and unclassified destinations are rejected. Recoverable Items restoration is not supported.",
		SeeDocs:     []string{"concepts#mail-filing-and-recovery"},
		Handler:     wrapWrite("mail.restore_message", "write", tools.NewHandleRestoreMessage(retryCfg, c.timeout, c.referenceCodec)),
		Annotations: []mcp.ToolOption{
			mcp.WithReadOnlyHintAnnotation(false), mcp.WithDestructiveHintAnnotation(false),
			mcp.WithIdempotentHintAnnotation(false), mcp.WithOpenWorldHintAnnotation(true),
		},
		Schema: append(referencedMessageMutationSchema(), mcp.WithString("destination_folder_ref", mcp.Required(), mcp.Description("Same-target ordinary destination folder reference."))),
	}
}

// buildPermanentDeleteMessageVerb constructs the destructive, elicitation-
// gated Graph permanent-delete action.
func buildPermanentDeleteMessageVerb(c mailVerbsConfig, retryCfg graph.RetryConfig, wrapWrite func(string, string, mcpserver.ToolHandlerFunc) tools.Handler) tools.Verb {
	return tools.Verb{
		Name: "permanent_delete_message", Summary: "permanently delete one referenced message after human confirmation",
		Description: "Permanently deletes exactly one referenced own or shared-mail message after fresh MCP human elicitation. Outlook clients cannot recover it, although mailbox retention or legal hold may still apply. Requires the exact permanent_delete capability; ambiguous failures are never retried.",
		SeeDocs:     []string{"concepts#mail-filing-and-recovery"},
		Handler:     wrapWrite("mail.permanent_delete_message", "delete", tools.NewHandlePermanentDeleteMessage(retryCfg, c.timeout)),
		Annotations: []mcp.ToolOption{
			mcp.WithReadOnlyHintAnnotation(false), mcp.WithDestructiveHintAnnotation(true),
			mcp.WithIdempotentHintAnnotation(true), mcp.WithOpenWorldHintAnnotation(true),
		},
		Schema: referencedMessageMutationSchema(),
	}
}

// referencedMessageMutationSchema returns the common static selector schema
// for exact-reference own and shared message actions.
func referencedMessageMutationSchema() []mcp.ToolOption {
	return []mcp.ToolOption{
		mcp.WithString("message_ref", mcp.Required(), mcp.Description("Target-bound source message reference.")),
		mcp.WithString("shared_resource", mcp.Description("Configured shared-mail alias. Omit for own mail.")),
		mcp.WithString("account", mcp.Description("Account label or UPN to use.")),
	}
}
