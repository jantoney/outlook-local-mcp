package auth

import (
	"slices"
	"testing"

	"github.com/desek/outlook-local-mcp/internal/resource"
)

// TestScopesForAccountConfig verifies exact union composition across own mail
// and independent shared-calendar targets without duplicate broad scopes.
func TestScopesForAccountConfig(t *testing.T) {
	t.Parallel()

	policy := MailActionPolicy{Read: true}
	aliases := []resource.CalendarAlias{
		{Profile: resource.CalendarProfileRead},
		{Profile: resource.CalendarProfileManage},
		{Profile: resource.CalendarProfileRead},
	}
	account := AccountConfig{
		CalendarPolicy:  func() *CalendarPolicy { p := CalendarPolicyManage; return &p }(),
		MailPolicy:      &policy,
		CalendarAliases: &aliases,
	}
	want := []string{"User.Read", "Calendars.ReadWrite", "Calendars.Read.Shared", "Calendars.ReadWrite.Shared", "Mail.Read"}
	if got := ScopesForAccountConfig(account); !slices.Equal(got, want) {
		t.Fatalf("ScopesForAccountConfig() = %v, want %v", got, want)
	}
}
