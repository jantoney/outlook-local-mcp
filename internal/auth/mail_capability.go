package auth

// MailCapability identifies one independently authorized mail action.
type MailCapability string

const (
	// MailCapabilityRead permits mail, folder, conversation, and attachment reads.
	MailCapabilityRead MailCapability = "read"
	// MailCapabilityDraft permits draft lifecycle and attachment writes.
	MailCapabilityDraft MailCapability = "draft"
	// MailCapabilityMove permits moves to ordinary folders.
	MailCapabilityMove MailCapability = "move"
	// MailCapabilityArchive permits moves to Outlook's Archive folder.
	MailCapabilityArchive MailCapability = "archive"
	// MailCapabilityTrash permits reversible moves to Deleted Items.
	MailCapabilityTrash MailCapability = "trash"
	// MailCapabilityRestore permits restoration from Deleted Items.
	MailCapabilityRestore MailCapability = "restore"
	// MailCapabilityPermanentDelete permits separately confirmed permanent deletion.
	MailCapabilityPermanentDelete MailCapability = "permanent_delete"
	// MailCapabilitySend permits delivery of an existing reviewed draft.
	MailCapabilitySend MailCapability = "send"
)
