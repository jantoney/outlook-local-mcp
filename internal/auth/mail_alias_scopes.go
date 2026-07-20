package auth

import "github.com/desek/outlook-local-mcp/internal/resource"

const (
	mailReadSharedScope      = "Mail.Read.Shared"
	mailReadWriteSharedScope = "Mail.ReadWrite.Shared"
	mailSendSharedScope      = "Mail.Send.Shared"
)

// ScopesForSharedMailPolicy returns the least broad delegated shared-mail
// scopes required by one alias policy. The zero policy contributes no scope;
// callers must still enforce target-local authorization before Graph traffic.
func ScopesForSharedMailPolicy(policy MailActionPolicy) []string {
	scopes := []string{}
	if policy.Read {
		scopes = append(scopes, mailReadSharedScope)
	}
	if policy.Draft || policy.Move || policy.Archive || policy.Trash ||
		policy.Restore || policy.PermanentDelete || policy.Send {
		scopes = append(scopes, mailReadWriteSharedScope)
	}
	if policy.Send {
		scopes = append(scopes, mailSendSharedScope)
	}
	return OAuthScopeUnion(scopes)
}

// ScopesForMailAliases returns the deterministic deduplicated shared-mail
// scope union required by every configured alias.
func ScopesForMailAliases(aliases []resource.MailAlias) []string {
	sets := make([][]string, 0, len(aliases))
	for _, alias := range aliases {
		sets = append(sets, ScopesForSharedMailPolicy(alias.Policy))
	}
	return OAuthScopeUnion(sets...)
}

// ScopesForAccountEntry returns the configured account-wide delegated scope
// union for own mail, shared calendars, and shared mail aliases.
func ScopesForAccountEntry(entry *AccountEntry) []string {
	if entry == nil {
		return nil
	}
	return OAuthScopeUnion(
		ScopesForMailPolicy(entry.MailPolicy),
		ScopesForCalendarAliases(entry.CalendarAliases),
		ScopesForMailAliases(entry.MailAliases),
	)
}
