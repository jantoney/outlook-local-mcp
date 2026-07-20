package resource

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

// MailActionPolicy is the persisted independent capability set for one mail
// target. Its zero value denies every mail action, including permanent deletion.
type MailActionPolicy struct {
	// Read permits mail and folder reads.
	Read bool `json:"read"`
	// Draft permits draft lifecycle and attachment writes.
	Draft bool `json:"draft"`
	// Move permits moves to ordinary folders.
	Move bool `json:"move"`
	// Archive permits semantic Outlook Archive moves.
	Archive bool `json:"archive"`
	// Trash permits reversible moves to Deleted Items.
	Trash bool `json:"trash"`
	// Restore permits restoration from Deleted Items.
	Restore bool `json:"restore"`
	// PermanentDelete permits separately confirmed permanent deletion.
	PermanentDelete bool `json:"permanent_delete"`
	// Send permits delivery of an existing reviewed draft.
	Send bool `json:"send"`
}

// Allows reports whether the policy enables capability. It returns false for
// unknown values so future or malformed capabilities fail closed.
func (p MailActionPolicy) Allows(capability MailCapability) bool {
	switch capability {
	case MailCapabilityRead:
		return p.Read
	case MailCapabilityDraft:
		return p.Draft
	case MailCapabilityMove:
		return p.Move
	case MailCapabilityArchive:
		return p.Archive
	case MailCapabilityTrash:
		return p.Trash
	case MailCapabilityRestore:
		return p.Restore
	case MailCapabilityPermanentDelete:
		return p.PermanentDelete
	case MailCapabilitySend:
		return p.Send
	default:
		return false
	}
}
