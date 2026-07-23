package auth

import "github.com/desek/outlook-local-mcp/internal/resource"

// RequiredScopes returns the deterministic delegated scope union for one
// account's complete configured policy. User.Read is always required for
// signed-in identity validation; every optional resource scope is derived from
// explicit local policy. The returned slice is newly allocated.
func RequiredScopes(calendarPolicy CalendarPolicy, mailPolicy MailActionPolicy, calendarAliases []resource.CalendarAlias, mailAliases []resource.MailAlias) []string {
	return OAuthScopeUnion(
		[]string{userReadScope},
		ScopesForCalendarPolicy(calendarPolicy),
		ScopesForMailPolicy(mailPolicy),
		ScopesForCalendarAliases(calendarAliases),
		ScopesForMailAliases(mailAliases),
	)
}
