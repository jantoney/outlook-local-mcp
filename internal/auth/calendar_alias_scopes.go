package auth

import "github.com/desek/outlook-local-mcp/internal/resource"

const (
	calendarReadSharedScope      = "Calendars.Read.Shared"
	calendarReadWriteSharedScope = "Calendars.ReadWrite.Shared"
)

// ScopesForCalendarProfile returns the exact delegated Graph scope required by
// one shared-calendar profile. Off and malformed profiles contribute no scope.
func ScopesForCalendarProfile(profile resource.CalendarProfile) []string {
	switch profile {
	case resource.CalendarProfileRead:
		return []string{calendarReadSharedScope}
	case resource.CalendarProfileManage:
		return []string{calendarReadWriteSharedScope}
	default:
		return nil
	}
}

// ScopesForCalendarAliases returns the deterministic deduplicated scope union
// required by all configured calendar aliases. It performs no mutation.
func ScopesForCalendarAliases(aliases []resource.CalendarAlias) []string {
	sets := make([][]string, 0, len(aliases))
	for _, alias := range aliases {
		sets = append(sets, ScopesForCalendarProfile(alias.Profile))
	}
	return OAuthScopeUnion(sets...)
}

// ScopesForAccountConfig returns the account-wide delegated scope union for
// own-resource policy and every configured shared calendar and mail target.
func ScopesForAccountConfig(account AccountConfig) []string {
	policy := MailActionPolicy{}
	if account.MailPolicy != nil {
		policy = *account.MailPolicy
	} else if profile, err := ParseMailProfile(account.MailProfile); err == nil {
		policy = MailPolicyFromProfile(profile)
	}
	var aliases []resource.CalendarAlias
	if account.CalendarAliases != nil {
		aliases = *account.CalendarAliases
	}
	var mailAliases []resource.MailAlias
	if account.MailAliases != nil {
		mailAliases = *account.MailAliases
	}
	return RequiredScopes(calendarPolicyFromConfig(account), policy, aliases, mailAliases)
}

// calendarPolicyFromConfig returns the persisted own-calendar policy. Missing
// legacy input fails closed to off until migration persists an explicit value.
func calendarPolicyFromConfig(account AccountConfig) CalendarPolicy {
	if account.CalendarPolicy == nil {
		return CalendarPolicyOff
	}
	return *account.CalendarPolicy
}
