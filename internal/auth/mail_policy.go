package auth

import "github.com/desek/outlook-local-mcp/internal/resource"

// MailActionPolicy aliases the resource-neutral independent mail policy.
type MailActionPolicy = resource.MailActionPolicy

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
