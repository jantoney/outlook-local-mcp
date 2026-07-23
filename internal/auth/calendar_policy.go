package auth

import "fmt"

// CalendarPolicy defines the exact local access granted to an account's own
// calendar. OAuth consent is necessary but never sufficient: runtime guards
// also enforce this policy before any Microsoft Graph request is sent.
type CalendarPolicy string

const (
	// CalendarPolicyOff disables all access to the account's own calendar.
	CalendarPolicyOff CalendarPolicy = "off"
	// CalendarPolicyRead permits read-only access to the account's own calendar.
	CalendarPolicyRead CalendarPolicy = "read"
	// CalendarPolicyManage permits reads and mutations on the account's own calendar.
	CalendarPolicyManage CalendarPolicy = "manage"
)

// ParseCalendarPolicy validates value and returns its normalized policy.
// It returns an error for empty or unknown values and has no side effects.
func ParseCalendarPolicy(value string) (CalendarPolicy, error) {
	policy := CalendarPolicy(value)
	switch policy {
	case CalendarPolicyOff, CalendarPolicyRead, CalendarPolicyManage:
		return policy, nil
	default:
		return CalendarPolicyOff, fmt.Errorf("invalid calendar policy %q: expected off, read, or manage", value)
	}
}

// ScopesForCalendarPolicy returns the least-privileged delegated Graph scope
// required by policy. The returned slice is newly allocated. The function does
// not authorize an operation and performs no I/O.
func ScopesForCalendarPolicy(policy CalendarPolicy) []string {
	switch policy {
	case CalendarPolicyRead:
		return []string{"Calendars.Read"}
	case CalendarPolicyManage:
		return []string{"Calendars.ReadWrite"}
	default:
		return nil
	}
}
