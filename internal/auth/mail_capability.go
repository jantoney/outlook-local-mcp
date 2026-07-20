package auth

import "github.com/desek/outlook-local-mcp/internal/resource"

// MailCapability aliases the resource-neutral mail action discriminant.
type MailCapability = resource.MailCapability

const (
	// MailCapabilityRead permits mail, folder, conversation, and attachment reads.
	MailCapabilityRead = resource.MailCapabilityRead
	// MailCapabilityDraft permits draft lifecycle and attachment writes.
	MailCapabilityDraft = resource.MailCapabilityDraft
	// MailCapabilityMove permits moves to ordinary folders.
	MailCapabilityMove = resource.MailCapabilityMove
	// MailCapabilityArchive permits moves to Outlook's Archive folder.
	MailCapabilityArchive = resource.MailCapabilityArchive
	// MailCapabilityTrash permits reversible moves to Deleted Items.
	MailCapabilityTrash = resource.MailCapabilityTrash
	// MailCapabilityRestore permits restoration from Deleted Items.
	MailCapabilityRestore = resource.MailCapabilityRestore
	// MailCapabilityPermanentDelete permits separately confirmed permanent deletion.
	MailCapabilityPermanentDelete = resource.MailCapabilityPermanentDelete
	// MailCapabilitySend permits delivery of an existing reviewed draft.
	MailCapabilitySend = resource.MailCapabilitySend
)
