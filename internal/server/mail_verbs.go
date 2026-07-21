// Package server — this file builds the mail domain verb slice for the
// aggregate "mail" MCP tool (CR-0060 Phase 3c).
//
// It lives in the server package rather than tools to avoid the import cycle
// that would arise from tools importing tools/help (which itself imports tools).
//
// All mail verbs are registered at startup. Global flags derive the default
// account profile; per-account capability middleware authorizes each call.
package server

import (
	"time"

	"github.com/desek/outlook-local-mcp/internal/audit"
	"github.com/desek/outlook-local-mcp/internal/auth"
	"github.com/desek/outlook-local-mcp/internal/config"
	"github.com/desek/outlook-local-mcp/internal/graph"
	"github.com/desek/outlook-local-mcp/internal/observability"
	"github.com/desek/outlook-local-mcp/internal/resource"
	"github.com/desek/outlook-local-mcp/internal/tools"
	"github.com/desek/outlook-local-mcp/internal/tools/help"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"go.opentelemetry.io/otel/trace"
)

// mailVerbsConfig holds the dependencies required to build the mail domain verb
// slice. All fields are captured at server start.
type mailVerbsConfig struct {
	// registry supplies the selected account's current target allowlist.
	registry *auth.AccountRegistry

	// referenceCodec signs and verifies restart-stable mail provenance.
	referenceCodec *resource.ReferenceCodec

	// retryCfg is the Graph API retry configuration applied to all mail handlers.
	retryCfg graph.RetryConfig

	// timeout is the maximum duration for a single Graph API call.
	timeout time.Duration

	// cfg is the full server configuration, used for feature-flag gating and
	// derived values such as MaxAttachmentSizeBytes and ProvenanceTag.
	cfg config.Config

	// provenancePropertyID is the fully-qualified MAPI extended property ID for
	// provenance tagging, built once at startup. Empty string disables tagging.
	provenancePropertyID string

	// m is the ToolMetrics instance for observability instrumentation.
	m *observability.ToolMetrics

	// tracer is the OTEL tracer for span creation.
	tracer trace.Tracer

	// authMW is the authentication middleware factory applied to every mail verb.
	authMW func(mcpserver.ToolHandlerFunc) mcpserver.ToolHandlerFunc

	// accountResolverMW is the account-resolver middleware applied to every mail
	// verb (mail tools resolve the Graph client via AccountResolver).
	accountResolverMW func(mcpserver.ToolHandlerFunc) mcpserver.ToolHandlerFunc

	// readOnly controls whether write verbs are blocked by ReadOnlyGuard.
	readOnly bool
}

// buildMailVerbs constructs the ordered []tools.Verb slice for the mail domain
// aggregate tool and returns a pointer to an initially empty VerbRegistry.
//
// All supported verbs are registered unconditionally. Their handlers are
// guarded by requiredMailProfile after account resolution, so discovery is
// stable while authorization follows the selected account.
//
// Each verb's Handler is pre-wrapped with authMW, accountResolverMW,
// observability, and audit middleware using the fully-qualified identity
// "mail.<verb>" per CR-0060 FR-13 and FR-14. Write verbs additionally include
// ReadOnlyGuard between observability and audit.
//
// The returned registry pointer is empty at the time of return. The caller
// MUST call RegisterDomainTool with the returned verbs, then assign the returned
// VerbRegistry back through the pointer so that the help verb can introspect
// all registered verbs at call time.
//
// Parameters:
//   - c: mailVerbsConfig with all required dependencies.
//
// Returns:
//   - verbs: ordered Verb slice for use with RegisterDomainTool.
//   - registryPtr: pointer whose value is assigned after registration.
func buildMailVerbs(c mailVerbsConfig) ([]tools.Verb, *tools.VerbRegistry) {
	empty := make(tools.VerbRegistry)
	registryPtr := &empty

	wrapTarget := func(guard TargetGuardConfig) func(string, string, mcpserver.ToolHandlerFunc) tools.Handler {
		return func(name, auditOp string, h mcpserver.ToolHandlerFunc) tools.Handler {
			h = ResolvedTargetGuard(c.registry, guard, c.referenceCodec, h)
			return tools.Handler(c.accountResolverMW(c.authMW(observability.WithObservability(name, c.m, c.tracer, audit.AuditWrap(name, auditOp, h)))))
		}
	}
	wrapTargetWrite := func(guard TargetGuardConfig) func(string, string, mcpserver.ToolHandlerFunc) tools.Handler {
		return func(name, auditOp string, h mcpserver.ToolHandlerFunc) tools.Handler {
			h = ResolvedTargetGuard(c.registry, guard, c.referenceCodec, h)
			return tools.Handler(c.accountResolverMW(c.authMW(observability.WithObservability(name, c.m, c.tracer, ReadOnlyGuard(name, c.readOnly, audit.AuditWrap(name, auditOp, h))))))
		}
	}

	rc := c.retryCfg

	verbs := []tools.Verb{
		help.NewHelpVerb(registryPtr),
		buildListFoldersVerb(c, rc, wrapTarget(mailSharedReadGuard())),
		buildListMessagesVerb(c, rc, wrapTarget(mailSharedFolderReadGuard())),
		buildGetMessageVerb(c, rc, wrapTarget(mailSharedMessageReadGuard())),
		buildSearchMessagesVerb(c, rc, wrapTarget(mailSharedFolderReadGuard())),
		buildGetConversationVerb(c, rc, wrapTarget(mailSharedParentMessageGuard("message_ref"))),
		buildListAttachmentsVerb(c, rc, wrapTarget(mailSharedParentMessageGuard("message_ref"))),
		buildGetAttachmentVerb(c, rc, wrapTarget(mailSharedParentMessageGuard("message_ref"))),
		buildCreateDraftVerb(c, rc, wrapTargetWrite(mailSharedDraftCreateGuard())),
		buildCreateReplyDraftVerb(c, rc, wrapTargetWrite(mailSharedDraftSourceGuard())),
		buildCreateForwardDraftVerb(c, rc, wrapTargetWrite(mailSharedDraftSourceGuard())),
		buildUpdateDraftVerb(c, rc, wrapTargetWrite(mailSharedDraftItemGuard())),
		buildDeleteDraftVerb(c, rc, wrapTargetWrite(mailSharedDraftItemGuard())),
		buildAddAttachmentVerb(c, rc, wrapTargetWrite(mailSharedDraftItemGuard())),
		buildMoveMessageVerb(c, rc, wrapTargetWrite(mailMoveMessageGuard())),
		buildArchiveMessageVerb(c, wrapTargetWrite(mailReferencedMessageMutationGuard(resource.MailCapabilityArchive))),
		buildTrashMessageVerb(c, wrapTargetWrite(mailReferencedMessageMutationGuard(resource.MailCapabilityTrash))),
		buildRestoreMessageVerb(c, rc, wrapTargetWrite(mailReferencedMessageMutationGuard(resource.MailCapabilityRestore))),
		buildPermanentDeleteMessageVerb(c, rc, wrapTargetWrite(mailReferencedMessageMutationGuard(resource.MailCapabilityPermanentDelete))),
		buildSendDraftVerb(c, rc, wrapTargetWrite(mailSendDraftGuard())),
	}
	for index := range verbs {
		if verbs[index].Name != "help" {
			verbs[index].RequiredCapability = string(requiredMailCapability("mail." + verbs[index].Name))
		}
	}

	return verbs, registryPtr
}

// requiredMailCapability returns the exact own-mail action permitted to invoke
// a mail verb. The static map keeps schema discovery independent of connected
// accounts while authorization remains target-specific at runtime.
func requiredMailCapability(name string) auth.MailCapability {
	switch name {
	case "mail.send_draft":
		return auth.MailCapabilitySend
	case "mail.create_draft", "mail.create_reply_draft", "mail.create_forward_draft",
		"mail.update_draft", "mail.delete_draft", "mail.add_attachment":
		return auth.MailCapabilityDraft
	case "mail.move_message":
		return auth.MailCapabilityMove
	case "mail.archive_message":
		return auth.MailCapabilityArchive
	case "mail.trash_message":
		return auth.MailCapabilityTrash
	case "mail.restore_message":
		return auth.MailCapabilityRestore
	case "mail.permanent_delete_message":
		return auth.MailCapabilityPermanentDelete
	default:
		return auth.MailCapabilityRead
	}
}

// buildMoveMessageVerb constructs the exact-capability ordinary filing verb.
func buildMoveMessageVerb(c mailVerbsConfig, rc graph.RetryConfig, wrapWrite func(string, string, mcpserver.ToolHandlerFunc) tools.Handler) tools.Verb {
	return tools.Verb{
		Name:        "move_message",
		Summary:     "move a referenced message to a referenced ordinary folder",
		Description: "Moves one referenced own or shared-mail message to a same-target folder reference that was authoritatively classified as ordinary. Archive, Deleted Items, Drafts, Outbox, recoverable-items, and every reserved or unclassified destination are rejected locally. Returns the destination message's new reference because Graph move can change IDs.",
		SeeDocs:     []string{"concepts#mail-filing-and-recovery"},
		Handler:     wrapWrite("mail.move_message", "write", tools.NewHandleMoveMessage(rc, c.timeout, c.referenceCodec)),
		Annotations: []mcp.ToolOption{
			mcp.WithReadOnlyHintAnnotation(false), mcp.WithDestructiveHintAnnotation(false),
			mcp.WithIdempotentHintAnnotation(false), mcp.WithOpenWorldHintAnnotation(true),
		},
		Schema: []mcp.ToolOption{
			mcp.WithString("message_ref", mcp.Required(), mcp.Description("Target-bound source message reference.")),
			mcp.WithString("destination_folder_ref", mcp.Required(), mcp.Description("Same-target folder reference classified for ordinary filing.")),
			mcp.WithString("shared_resource", mcp.Description("Configured shared-mail alias. Omit for own mail.")),
			mcp.WithString("account", mcp.Description("Account label or UPN to use.")),
		},
	}
}

// buildSendDraftVerb constructs the elicitation-gated existing-draft send verb.
func buildSendDraftVerb(c mailVerbsConfig, rc graph.RetryConfig, wrapWrite func(string, string, mcpserver.ToolHandlerFunc) tools.Handler) tools.Verb {
	return tools.Verb{
		Name:        "send_draft",
		Summary:     "send an existing draft after explicit human confirmation",
		Description: "Sends only an existing draft after MCP human elicitation and requires the exact send capability; there is no boolean confirmation parameter. Own mail retains message_id behavior. Shared mail requires shared_resource plus draft_ref, complete owner-route review evidence, an unchanged post-confirmation snapshot, and one non-retried send POST. Exchange chooses Send As or Send on Behalf from configured rights.",
		SeeDocs:     []string{"concepts#independent-mail-action-policies", "concepts#confirmed-draft-send"},
		Handler:     wrapWrite("mail.send_draft", "send", tools.NewHandleSendDraft(rc, c.timeout, c.referenceCodec)),
		Annotations: []mcp.ToolOption{
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithIdempotentHintAnnotation(false),
			mcp.WithOpenWorldHintAnnotation(true),
		},
		Schema: []mcp.ToolOption{
			mcp.WithString("message_id", mcp.Description("Own-mail existing draft message ID. Rejected with shared_resource.")),
			mcp.WithString("draft_ref", mcp.Description("Target-bound existing draft reference required with shared_resource.")),
			mcp.WithString("shared_resource", mcp.Description("Configured shared-mail alias. Omit for own mail.")),
			mcp.WithString("account", mcp.Description("Account label or UPN to use.")),
		},
	}
}

// buildAddAttachmentVerb constructs the draft-only local attachment verb.
func buildAddAttachmentVerb(c mailVerbsConfig, rc graph.RetryConfig, wrapWrite func(string, string, mcpserver.ToolHandlerFunc) tools.Handler) tools.Verb {
	return tools.Verb{
		Name:        "add_attachment",
		Summary:     "attach one allowlisted local file to an own or shared draft",
		Description: "Adds one local file to an existing draft. Shared calls require shared_resource plus draft_ref and support direct files below 3 MiB through the exact owner route. Own drafts retain resumable upload through 150 MiB. The canonical path must be inside OUTLOOK_MCP_ATTACHMENT_ROOTS. Requires the exact draft capability.",
		SeeDocs:     []string{"concepts#independent-mail-action-policies", "concepts#local-draft-attachments"},
		Handler:     wrapWrite("mail.add_attachment", "write", tools.NewHandleAddAttachment(rc, c.timeout, c.cfg.AttachmentRoots, nil, c.referenceCodec)),
		Annotations: []mcp.ToolOption{
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithIdempotentHintAnnotation(false),
			mcp.WithOpenWorldHintAnnotation(true),
		},
		Schema: []mcp.ToolOption{
			mcp.WithString("message_id", mcp.Description("Own-mail draft message ID. Rejected with shared_resource.")),
			mcp.WithString("draft_ref", mcp.Description("Target-bound shared draft reference.")),
			mcp.WithString("shared_resource", mcp.Description("Configured shared-mail alias. Omit for own mail.")),
			mcp.WithString("file_path", mcp.Required(), mcp.Description("Local file path inside a configured attachment root.")),
			mcp.WithString("account", mcp.Description("Account label or UPN to use.")),
		},
	}
}

// buildListFoldersVerb constructs the list_folders Verb.
func buildListFoldersVerb(c mailVerbsConfig, rc graph.RetryConfig, wrap func(string, string, mcpserver.ToolHandlerFunc) tools.Handler) tools.Verb {
	return tools.Verb{
		Name:        "list_folders",
		Summary:     "list mail folders (Inbox, Sent, Drafts, etc.) with unread and total counts",
		Description: "Returns mail folders with counts and shared read references. Set include_refs=true to resolve well-known reserved folders and emit signed ordinary/reserved destination classification for move_message on either own or shared mail. Existing own output is unchanged by default.",
		SeeDocs:     []string{"concepts#independent-mail-action-policies", "concepts#shared-mail-aliases"},
		Handler:     wrap("mail.list_folders", "read", tools.NewHandleListMailFolders(rc, c.timeout, c.referenceCodec)),
		Annotations: []mcp.ToolOption{
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithIdempotentHintAnnotation(true),
			mcp.WithOpenWorldHintAnnotation(true),
		},
		Schema: []mcp.ToolOption{
			mcp.WithString("shared_resource", mcp.Description("Optional organizational shared-mail alias with read capability.")),
			mcp.WithBoolean("include_refs", mcp.Description("Emit target-bound destination references and authoritative ordinary/reserved classification for move_message.")),
			mcp.WithString("account",
				mcp.Description("Account label or UPN to use. Omit to auto-select the default account."),
			),
			mcp.WithNumber("max_results",
				mcp.Description("Maximum number of folders to return (default 25)."),
				mcp.Min(1),
			),
			mcp.WithString("output",
				mcp.Description("Output mode: 'text' (default), 'summary', or 'raw'."),
				mcp.Enum("text", "summary", "raw"),
			),
		},
	}
}

// buildListMessagesVerb constructs the list_messages Verb.
func buildListMessagesVerb(c mailVerbsConfig, rc graph.RetryConfig, wrap func(string, string, mcpserver.ToolHandlerFunc) tools.Handler) tools.Verb {
	return tools.Verb{
		Name:        "list_messages",
		Summary:     "list messages in a folder or across all folders; filter by date, sender, thread",
		Description: "Lists messages in an own or organizational shared mailbox. Shared results include references; set include_refs=true to opt into the same signed references for own-mail move_message without changing default own output. Raw keeps provenance in a sidecar. Results include bodyPreview; full body requires get_message output=raw.",
		Examples: []tools.Example{
			{Args: map[string]any{"folder_id": "Inbox", "is_read": false}, Comment: "list unread messages in inbox"},
			{Args: map[string]any{"from": "alice@contoso.com", "max_results": 10}, Comment: "list recent messages from a sender"},
		},
		SeeDocs: []string{"concepts#output-tiers", "concepts#independent-mail-action-policies", "concepts#shared-mail-aliases"},
		Handler: wrap("mail.list_messages", "read", tools.NewHandleListMessages(rc, c.timeout, c.provenancePropertyID, c.referenceCodec)),
		Annotations: []mcp.ToolOption{
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithIdempotentHintAnnotation(true),
			mcp.WithOpenWorldHintAnnotation(true),
		},
		Schema: []mcp.ToolOption{
			mcp.WithString("shared_resource", mcp.Description("Optional organizational shared-mail alias with read capability.")),
			mcp.WithBoolean("include_refs", mcp.Description("Emit target-bound message references for own-mail move_message; shared reads already include them.")),
			mcp.WithString("folder_id",
				mcp.Description("Own-mail folder ID. Rejected with shared_resource; use folder_ref."),
			),
			mcp.WithString("folder_ref", mcp.Description("Optional signed folder reference from shared list_folders.")),
			mcp.WithString("start_datetime",
				mcp.Description("Start of date range (ISO 8601, e.g. 2026-03-12T00:00:00Z). Filters by receivedDateTime >=."),
			),
			mcp.WithString("end_datetime",
				mcp.Description("End of date range (ISO 8601). Filters by receivedDateTime <=."),
			),
			mcp.WithString("from",
				mcp.Description("Sender email address to filter by (e.g. alice@contoso.com)."),
			),
			mcp.WithString("conversation_id",
				mcp.Description("Conversation ID to retrieve all messages in a thread."),
			),
			mcp.WithBoolean("is_read",
				mcp.Description("Filter by read/unread state. Omit to include both."),
			),
			mcp.WithBoolean("is_draft",
				mcp.Description("Filter by draft state. Omit to include both."),
			),
			mcp.WithBoolean("has_attachments",
				mcp.Description("Filter by attachment presence. Omit to include both."),
			),
			mcp.WithString("importance",
				mcp.Description("Filter by message importance."),
				mcp.Enum("low", "normal", "high"),
			),
			mcp.WithString("flag_status",
				mcp.Description("Filter by follow-up flag status."),
				mcp.Enum("notFlagged", "flagged", "complete"),
			),
			mcp.WithBoolean("provenance",
				mcp.Description("Filter to messages created by this MCP server (requires provenance tagging)."),
			),
			mcp.WithNumber("max_results",
				mcp.Description("Maximum number of messages to return (default 25, max 100)."),
				mcp.Min(1),
				mcp.Max(100),
			),
			mcp.WithString("timezone",
				mcp.Description("IANA timezone name for the Prefer: outlook.timezone header."),
			),
			mcp.WithString("account",
				mcp.Description("Account label or UPN to use. Omit to auto-select the default account."),
			),
			mcp.WithString("output",
				mcp.Description("Output mode: 'text' (default), 'summary', or 'raw'."),
				mcp.Enum("text", "summary", "raw"),
			),
		},
	}
}

// buildGetMessageVerb constructs the get_message Verb.
func buildGetMessageVerb(c mailVerbsConfig, rc graph.RetryConfig, wrap func(string, string, mcpserver.ToolHandlerFunc) tools.Handler) tools.Verb {
	return tools.Verb{
		Name:        "get_message",
		Summary:     "get full message details by ID; bodyPreview by default, full body via output=raw",
		Description: "Fetches one own or organizational shared-mail message. Shared calls select shared_resource and require the target-bound resource_ref returned by list_messages; alias-plus-message_id is rejected. Text and summary include renewed provenance. Raw preserves Graph-derived message data beside provenance and includes the full HTML body and headers.",
		SeeDocs:     []string{"concepts#output-tiers", "concepts#shared-mail-aliases", "troubleshooting#shared-mail-reference-rejected"},
		Handler:     wrap("mail.get_message", "read", tools.NewHandleGetMessage(rc, c.timeout, c.provenancePropertyID, c.referenceCodec)),
		Annotations: []mcp.ToolOption{
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithIdempotentHintAnnotation(true),
			mcp.WithOpenWorldHintAnnotation(true),
		},
		Schema: []mcp.ToolOption{
			mcp.WithString("shared_resource", mcp.Description("Optional organizational shared-mail alias with read capability. Requires resource_ref.")),
			mcp.WithString("message_id",
				mcp.Description("Own-mail message ID. Required without shared_resource and rejected for shared reads."),
			),
			mcp.WithString("resource_ref", mcp.Description("Signed target-bound message reference required with shared_resource.")),
			mcp.WithString("account",
				mcp.Description("Account label or UPN to use. Omit to auto-select the default account."),
			),
			mcp.WithString("output",
				mcp.Description("Output mode: 'text' (default), 'summary', or 'raw' (includes full HTML body and headers)."),
				mcp.Enum("text", "summary", "raw"),
			),
		},
	}
}

// buildSearchMessagesVerb constructs the search_messages Verb.
func buildSearchMessagesVerb(c mailVerbsConfig, rc graph.RetryConfig, wrap func(string, string, mcpserver.ToolHandlerFunc) tools.Handler) tools.Verb {
	return tools.Verb{
		Name:        "search_messages",
		Summary:     "full-text KQL search across messages; ranked by relevance, not chronologically",
		Description: "Searches own or organizational shared mail using KQL. Shared results include references; set include_refs=true to opt into own-mail references for move_message. Raw keeps provenance in a sidecar. Results are relevance-ranked and include bodyPreview; full body requires get_message output=raw.",
		Examples: []tools.Example{
			{Args: map[string]any{"query": "subject:\"quarterly review\""}, Comment: "find messages with a specific subject"},
			{Args: map[string]any{"query": "from:alice@contoso.com hasAttachments:true"}, Comment: "find messages with attachments from a sender"},
		},
		SeeDocs: []string{"concepts#output-tiers", "concepts#shared-mail-aliases"},
		Handler: wrap("mail.search_messages", "read", tools.NewHandleSearchMessages(rc, c.timeout, c.referenceCodec)),
		Annotations: []mcp.ToolOption{
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithIdempotentHintAnnotation(true),
			mcp.WithOpenWorldHintAnnotation(true),
		},
		Schema: []mcp.ToolOption{
			mcp.WithString("shared_resource", mcp.Description("Optional organizational shared-mail alias with read capability.")),
			mcp.WithBoolean("include_refs", mcp.Description("Emit target-bound message references for own-mail move_message; shared reads already include them.")),
			mcp.WithString("query",
				mcp.Required(),
				mcp.Description("KQL search string (e.g. subject:\"Design Review\" from:alice@contoso.com)."),
			),
			mcp.WithString("folder_id",
				mcp.Description("Own-mail folder ID. Rejected with shared_resource; use folder_ref."),
			),
			mcp.WithString("folder_ref", mcp.Description("Optional signed shared-mail folder reference from list_folders.")),
			mcp.WithNumber("max_results",
				mcp.Description("Maximum number of messages to return (default 25, max 100)."),
				mcp.Min(1),
				mcp.Max(100),
			),
			mcp.WithString("account",
				mcp.Description("Account label or UPN to use. Omit to auto-select the default account."),
			),
			mcp.WithString("output",
				mcp.Description("Output mode: 'text' (default), 'summary', or 'raw'."),
				mcp.Enum("text", "summary", "raw"),
			),
		},
	}
}

// buildGetConversationVerb constructs the mail_read get_conversation Verb.
func buildGetConversationVerb(c mailVerbsConfig, rc graph.RetryConfig, wrap func(string, string, mcpserver.ToolHandlerFunc) tools.Handler) tools.Verb {
	return tools.Verb{
		Name:        "get_conversation",
		Summary:     "retrieve all messages in an email thread in chronological order",
		Description: "Retrieves all messages in a thread chronologically. Own mail accepts message_id or conversation_id. Shared mail requires shared_resource plus a target-bound message_ref, resolves the conversation in the configured owner view, and returns message references in text/summary or a raw provenance sidecar. Content previews are the default; full bodies require output=raw.",
		SeeDocs:     []string{"concepts#output-tiers", "concepts#shared-mail-aliases"},
		Handler:     wrap("mail.get_conversation", "read", tools.NewHandleGetConversation(rc, c.timeout, c.provenancePropertyID, c.referenceCodec)),
		Annotations: []mcp.ToolOption{
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithIdempotentHintAnnotation(true),
			mcp.WithOpenWorldHintAnnotation(true),
		},
		Schema: []mcp.ToolOption{
			mcp.WithString("shared_resource", mcp.Description("Optional organizational shared-mail alias with read capability. Requires message_ref.")),
			mcp.WithString("message_id",
				mcp.Description("Own-mail message ID. Rejected with shared_resource."),
			),
			mcp.WithString("message_ref", mcp.Description("Signed shared-mail message reference used to resolve the conversation.")),
			mcp.WithString("conversation_id",
				mcp.Description("Own-mail conversation ID. Shared mail requires message_ref instead."),
			),
			mcp.WithNumber("max_results",
				mcp.Description("Maximum number of messages to return (default 50, max 100)."),
				mcp.Min(1),
				mcp.Max(100),
			),
			mcp.WithString("account",
				mcp.Description("Account label or UPN to use. Omit to auto-select the default account."),
			),
			mcp.WithString("output",
				mcp.Description("Output mode: 'text' (default), 'summary', or 'raw'."),
				mcp.Enum("text", "summary", "raw"),
			),
		},
	}
}

// buildListAttachmentsVerb constructs the mail_read list_attachments Verb.
func buildListAttachmentsVerb(c mailVerbsConfig, rc graph.RetryConfig, wrap func(string, string, mcpserver.ToolHandlerFunc) tools.Handler) tools.Verb {
	return tools.Verb{
		Name:        "list_attachments",
		Summary:     "list attachment metadata (id, name, contentType, size) for a message",
		Description: "Lists attachment metadata for one own or shared-mail message. Shared calls require shared_resource plus the target-bound message_ref returned by a message read. Text and summary expose attachment_ref values for download; raw preserves Graph-derived metadata beside provenance.",
		SeeDocs:     []string{"concepts#independent-mail-action-policies", "concepts#shared-mail-aliases"},
		Handler:     wrap("mail.list_attachments", "read", tools.NewHandleListAttachments(rc, c.timeout, c.referenceCodec)),
		Annotations: []mcp.ToolOption{
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithIdempotentHintAnnotation(true),
			mcp.WithOpenWorldHintAnnotation(true),
		},
		Schema: []mcp.ToolOption{
			mcp.WithString("shared_resource", mcp.Description("Optional organizational shared-mail alias with read capability. Requires message_ref.")),
			mcp.WithString("message_id",
				mcp.Description("Own-mail parent message ID. Rejected with shared_resource."),
			),
			mcp.WithString("message_ref", mcp.Description("Signed target-bound parent message reference required with shared_resource.")),
			mcp.WithString("account",
				mcp.Description("Account label or UPN to use. Omit to auto-select the default account."),
			),
			mcp.WithString("output",
				mcp.Description("Output mode: 'text' (default), 'summary', or 'raw'."),
				mcp.Enum("text", "summary", "raw"),
			),
		},
	}
}

// buildGetAttachmentVerb constructs the mail_read get_attachment Verb.
func buildGetAttachmentVerb(c mailVerbsConfig, rc graph.RetryConfig, wrap func(string, string, mcpserver.ToolHandlerFunc) tools.Handler) tools.Verb {
	return tools.Verb{
		Name:        "get_attachment",
		Summary:     "download an attachment; returns metadata and base64 content up to the size limit",
		Description: "Downloads an own or shared-mail attachment and returns metadata plus base64 content. Shared calls require shared_resource, the target-bound parent message_ref, and the attachment_ref returned by list_attachments; raw IDs are rejected. Size limits remain enforced before content is returned.",
		SeeDocs:     []string{"concepts#independent-mail-action-policies", "concepts#shared-mail-aliases", "troubleshooting#shared-mail-reference-rejected"},
		Handler:     wrap("mail.get_attachment", "read", tools.NewHandleGetAttachment(rc, c.timeout, c.cfg.MaxAttachmentSizeBytes, c.referenceCodec)),
		Annotations: []mcp.ToolOption{
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithIdempotentHintAnnotation(true),
			mcp.WithOpenWorldHintAnnotation(true),
		},
		Schema: []mcp.ToolOption{
			mcp.WithString("shared_resource", mcp.Description("Optional organizational shared-mail alias with read capability. Requires message_ref and attachment_ref.")),
			mcp.WithString("message_id",
				mcp.Description("Own-mail parent message ID. Rejected with shared_resource."),
			),
			mcp.WithString("message_ref", mcp.Description("Signed target-bound parent message reference required with shared_resource.")),
			mcp.WithString("attachment_id",
				mcp.Description("Own-mail attachment ID. Rejected with shared_resource."),
			),
			mcp.WithString("attachment_ref", mcp.Description("Signed target-bound attachment reference required with shared_resource.")),
			mcp.WithString("account",
				mcp.Description("Account label or UPN to use. Omit to auto-select the default account."),
			),
			mcp.WithString("output",
				mcp.Description("Output mode: 'text' (default), 'summary', or 'raw'."),
				mcp.Enum("text", "summary", "raw"),
			),
		},
	}
}

// buildCreateDraftVerb constructs the draft-capability create_draft Verb.
func buildCreateDraftVerb(c mailVerbsConfig, rc graph.RetryConfig, wrapWrite func(string, string, mcpserver.ToolHandlerFunc) tools.Handler) tools.Verb {
	return tools.Verb{
		Name:        "create_draft",
		Summary:     "create a new email draft in the Drafts folder (not sent automatically)",
		Description: "Creates a new own or organizational shared-mail draft through the exact selected mailbox. Shared drafts return a target-bound draft_ref. Supports recipients, subject, body, and importance. Requires the exact draft capability; draft never implies send.",
		Examples: []tools.Example{
			{Args: map[string]any{"to_recipients": "alice@contoso.com", "subject": "Follow-up", "body": "Hi Alice..."}, Comment: "create a simple plain-text draft"},
		},
		SeeDocs: []string{"concepts#independent-mail-action-policies"},
		Handler: wrapWrite("mail.create_draft", "write", tools.NewHandleCreateDraft(rc, c.timeout, c.provenancePropertyID, c.referenceCodec)),
		Annotations: []mcp.ToolOption{
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithIdempotentHintAnnotation(false),
			mcp.WithOpenWorldHintAnnotation(true),
		},
		Schema: []mcp.ToolOption{
			mcp.WithString("to_recipients",
				mcp.Description("Comma-separated list of To recipient email addresses."),
			),
			mcp.WithString("cc_recipients",
				mcp.Description("Comma-separated list of Cc recipient email addresses."),
			),
			mcp.WithString("bcc_recipients",
				mcp.Description("Comma-separated list of Bcc recipient email addresses."),
			),
			mcp.WithString("subject",
				mcp.Description("Draft subject line."),
			),
			mcp.WithString("body",
				mcp.Description("Draft body content. Plain text unless content_type is 'html'."),
			),
			mcp.WithString("content_type",
				mcp.Description("Body content type: 'text' (default) or 'html'."),
				mcp.Enum("text", "html"),
			),
			mcp.WithString("importance",
				mcp.Description("Draft importance: low, normal, or high."),
			),
			mcp.WithString("account",
				mcp.Description("Account label or UPN to use. Omit to auto-select the default account."),
			),
			mcp.WithString("shared_resource", mcp.Description("Configured shared-mail alias. Omit for own mail.")),
		},
	}
}

// buildCreateReplyDraftVerb constructs the draft-capability create_reply_draft Verb.
func buildCreateReplyDraftVerb(c mailVerbsConfig, rc graph.RetryConfig, wrapWrite func(string, string, mcpserver.ToolHandlerFunc) tools.Handler) tools.Verb {
	return tools.Verb{
		Name:        "create_reply_draft",
		Summary:     "create a reply draft to an existing message preserving threading headers",
		Description: "Creates an own or organizational shared-mail reply draft through the source message's exact mailbox. Shared calls require message_ref and return draft_ref. Preserves threading headers and requires the exact draft capability.",
		SeeDocs:     []string{"concepts#independent-mail-action-policies"},
		Handler:     wrapWrite("mail.create_reply_draft", "write", tools.NewHandleCreateReplyDraft(rc, c.timeout, c.provenancePropertyID, c.referenceCodec)),
		Annotations: []mcp.ToolOption{
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithIdempotentHintAnnotation(false),
			mcp.WithOpenWorldHintAnnotation(true),
		},
		Schema: []mcp.ToolOption{
			mcp.WithString("message_id", mcp.Description("Own-mail source message ID. Rejected with shared_resource.")),
			mcp.WithString("message_ref", mcp.Description("Target-bound shared source-message reference.")),
			mcp.WithString("shared_resource", mcp.Description("Configured shared-mail alias. Omit for own mail.")),
			mcp.WithString("comment",
				mcp.Description("Optional reply body text prepended to the quoted original."),
			),
			mcp.WithBoolean("reply_all",
				mcp.Description("When true, reply to all original recipients (To + Cc). Default false."),
			),
			mcp.WithString("account",
				mcp.Description("Account label or UPN to use. Omit to auto-select the default account."),
			),
		},
	}
}

// buildCreateForwardDraftVerb constructs the draft-capability create_forward_draft Verb.
func buildCreateForwardDraftVerb(c mailVerbsConfig, rc graph.RetryConfig, wrapWrite func(string, string, mcpserver.ToolHandlerFunc) tools.Handler) tools.Verb {
	return tools.Verb{
		Name:        "create_forward_draft",
		Summary:     "create a forward draft of an existing message with new recipients",
		Description: "Creates an own or organizational shared-mail forward draft through the source message's exact mailbox. Shared calls require message_ref and return draft_ref. Requires the exact draft capability.",
		SeeDocs:     []string{"concepts#independent-mail-action-policies"},
		Handler:     wrapWrite("mail.create_forward_draft", "write", tools.NewHandleCreateForwardDraft(rc, c.timeout, c.provenancePropertyID, c.referenceCodec)),
		Annotations: []mcp.ToolOption{
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithIdempotentHintAnnotation(false),
			mcp.WithOpenWorldHintAnnotation(true),
		},
		Schema: []mcp.ToolOption{
			mcp.WithString("message_id", mcp.Description("Own-mail source message ID. Rejected with shared_resource.")),
			mcp.WithString("message_ref", mcp.Description("Target-bound shared source-message reference.")),
			mcp.WithString("shared_resource", mcp.Description("Configured shared-mail alias. Omit for own mail.")),
			mcp.WithString("to_recipients",
				mcp.Description("Comma-separated list of To recipient email addresses."),
			),
			mcp.WithString("comment",
				mcp.Description("Optional forward body text prepended to the quoted original."),
			),
			mcp.WithString("account",
				mcp.Description("Account label or UPN to use. Omit to auto-select the default account."),
			),
		},
	}
}

// buildUpdateDraftVerb constructs the draft-capability update_draft Verb.
func buildUpdateDraftVerb(c mailVerbsConfig, rc graph.RetryConfig, wrapWrite func(string, string, mcpserver.ToolHandlerFunc) tools.Handler) tools.Verb {
	return tools.Verb{
		Name:        "update_draft",
		Summary:     "update draft fields (PATCH semantics; non-draft messages rejected)",
		Description: "Updates an own or organizational shared-mail draft through its exact mailbox after verifying isDraft=true. Shared calls require draft_ref and return a renewed draft_ref. Requires the exact draft capability.",
		SeeDocs:     []string{"concepts#independent-mail-action-policies"},
		Handler:     wrapWrite("mail.update_draft", "write", tools.NewHandleUpdateDraft(rc, c.timeout, c.referenceCodec)),
		Annotations: []mcp.ToolOption{
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithIdempotentHintAnnotation(true),
			mcp.WithOpenWorldHintAnnotation(true),
		},
		Schema: []mcp.ToolOption{
			mcp.WithString("message_id", mcp.Description("Own-mail draft message ID. Rejected with shared_resource.")),
			mcp.WithString("draft_ref", mcp.Description("Target-bound shared draft reference.")),
			mcp.WithString("shared_resource", mcp.Description("Configured shared-mail alias. Omit for own mail.")),
			mcp.WithString("to_recipients",
				mcp.Description("Comma-separated list of To recipient email addresses (replaces existing)."),
			),
			mcp.WithString("cc_recipients",
				mcp.Description("Comma-separated list of Cc recipient email addresses (replaces existing)."),
			),
			mcp.WithString("bcc_recipients",
				mcp.Description("Comma-separated list of Bcc recipient email addresses (replaces existing)."),
			),
			mcp.WithString("subject",
				mcp.Description("New draft subject line."),
			),
			mcp.WithString("body",
				mcp.Description("New draft body content."),
			),
			mcp.WithString("content_type",
				mcp.Description("Body content type: 'text' or 'html'."),
				mcp.Enum("text", "html"),
			),
			mcp.WithString("importance",
				mcp.Description("New draft importance: low, normal, or high."),
			),
			mcp.WithString("account",
				mcp.Description("Account label or UPN to use. Omit to auto-select the default account."),
			),
		},
	}
}

// buildDeleteDraftVerb constructs the draft-capability delete_draft Verb.
func buildDeleteDraftVerb(c mailVerbsConfig, rc graph.RetryConfig, wrapWrite func(string, string, mcpserver.ToolHandlerFunc) tools.Handler) tools.Verb {
	return tools.Verb{
		Name:        "delete_draft",
		Summary:     "permanently delete a draft message (irreversible; non-draft messages rejected)",
		Description: "Permanently deletes an own or organizational shared-mail draft through its exact mailbox after verifying isDraft=true. Shared calls require draft_ref. Requires the exact draft capability.",
		SeeDocs:     []string{"concepts#independent-mail-action-policies"},
		Handler:     wrapWrite("mail.delete_draft", "delete", tools.NewHandleDeleteDraft(rc, c.timeout)),
		Annotations: []mcp.ToolOption{
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(true),
			mcp.WithIdempotentHintAnnotation(true),
			mcp.WithOpenWorldHintAnnotation(true),
		},
		Schema: []mcp.ToolOption{
			mcp.WithString("message_id", mcp.Description("Own-mail draft message ID. Rejected with shared_resource.")),
			mcp.WithString("draft_ref", mcp.Description("Target-bound shared draft reference.")),
			mcp.WithString("shared_resource", mcp.Description("Configured shared-mail alias. Omit for own mail.")),
			mcp.WithString("account",
				mcp.Description("Account label or UPN to use. Omit to auto-select the default account."),
			),
		},
	}
}

// mailToolAnnotations returns the conservative aggregate MCP annotations for
// the mail domain tool per CR-0060 FR-9 and AC-9.
//
// readOnlyHint is false because write verbs (create_draft, create_reply_draft,
// create_forward_draft, update_draft, delete_draft) may be present when
// MailManageEnabled is true. destructiveHint is true because delete_draft
// permanently removes a message. idempotentHint is false because create_draft,
// create_reply_draft, and create_forward_draft are non-idempotent.
// openWorldHint is true because all verbs call Microsoft Graph.
//
// Per FR-9 these values represent the most conservative annotation across all
// verbs that may be registered for the domain. They remain fixed at
// construction time and must be consistent across deployment configurations.
func mailToolAnnotations() []mcp.ToolOption {
	return []mcp.ToolOption{
		mcp.WithTitleAnnotation("Mail"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(true),
	}
}
