package auth

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

// MailPolicyFromProfile deterministically migrates one cumulative legacy
// profile to the exact independent actions that profile previously exposed.
// Newly introduced filing, recovery, and permanent-deletion actions stay off.
func MailPolicyFromProfile(profile MailProfile) MailActionPolicy {
	switch profile {
	case MailProfileRead:
		return MailActionPolicy{Read: true}
	case MailProfileManage:
		return MailActionPolicy{Read: true, Draft: true}
	case MailProfileSend:
		return MailActionPolicy{Read: true, Draft: true, Send: true}
	default:
		return MailActionPolicy{}
	}
}

// LegacyProfileForPolicy returns the narrowest legacy profile whose OAuth
// scopes cover policy. It exists only for transition code that still accepts a
// profile; authorization and persistence must use the policy itself.
func LegacyProfileForPolicy(policy MailActionPolicy) MailProfile {
	if policy.Send {
		return MailProfileSend
	}
	if policy.Draft || policy.Move || policy.Archive || policy.Trash ||
		policy.Restore || policy.PermanentDelete {
		return MailProfileManage
	}
	if policy.Read {
		return MailProfileRead
	}
	return MailProfileCalendarOnly
}
