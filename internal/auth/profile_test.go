package auth

import (
	"testing"

	"github.com/desek/outlook-local-mcp/internal/config"
)

// TestParseMailProfile verifies every supported legacy profile parses exactly.
func TestParseMailProfile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  MailProfile
	}{
		{"calendar_only", MailProfileCalendarOnly},
		{"mail_read", MailProfileRead},
		{"mail_manage", MailProfileManage},
		{"mail_send", MailProfileSend},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			got, err := ParseMailProfile(tt.input)
			if err != nil {
				t.Fatalf("ParseMailProfile(%q) error = %v", tt.input, err)
			}
			if got != tt.want {
				t.Fatalf("ParseMailProfile(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestParseMailProfileRejectsUnknown verifies unsupported profile names fail.
func TestParseMailProfileRejectsUnknown(t *testing.T) {
	t.Parallel()

	if _, err := ParseMailProfile("owner"); err == nil {
		t.Fatal("ParseMailProfile(owner) returned nil error")
	}
}

// TestMailProfileAllows verifies the legacy cumulative capability ordering.
func TestMailProfileAllows(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		actual   MailProfile
		required MailProfile
		want     bool
	}{
		{"send allows manage", MailProfileSend, MailProfileManage, true},
		{"manage allows read", MailProfileManage, MailProfileRead, true},
		{"read denies manage", MailProfileRead, MailProfileManage, false},
		{"calendar denies read", MailProfileCalendarOnly, MailProfileRead, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.actual.Allows(tt.required); got != tt.want {
				t.Fatalf("%q.Allows(%q) = %v, want %v", tt.actual, tt.required, got, tt.want)
			}
		})
	}
}

// TestScopesForProfile verifies each legacy profile's delegated scope union.
func TestScopesForProfile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		profile MailProfile
		want    []string
	}{
		{"calendar only", MailProfileCalendarOnly, []string{"User.Read", "Calendars.ReadWrite"}},
		{"mail read", MailProfileRead, []string{"User.Read", "Calendars.ReadWrite", "Mail.Read"}},
		{"mail manage", MailProfileManage, []string{"User.Read", "Calendars.ReadWrite", "Mail.ReadWrite"}},
		{"mail send", MailProfileSend, []string{"User.Read", "Calendars.ReadWrite", "Mail.ReadWrite", "Mail.Send"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := ScopesForProfile(tt.profile)
			if len(got) != len(tt.want) {
				t.Fatalf("ScopesForProfile(%q) = %v, want %v", tt.profile, got, tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Fatalf("ScopesForProfile(%q) = %v, want %v", tt.profile, got, tt.want)
				}
			}
		})
	}
}

// TestScopesForMailPolicy verifies OAuth scopes are the least broad set needed
// by an own-mail action policy and never substitute for local authorization.
func TestScopesForMailPolicy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		policy MailActionPolicy
		want   []string
	}{
		{"disabled", MailActionPolicy{}, []string{"User.Read", "Calendars.ReadWrite"}},
		{"read", MailActionPolicy{Read: true}, []string{"User.Read", "Calendars.ReadWrite", "Mail.Read"}},
		{"draft", MailActionPolicy{Draft: true}, []string{"User.Read", "Calendars.ReadWrite", "Mail.ReadWrite"}},
		{"filing", MailActionPolicy{Archive: true}, []string{"User.Read", "Calendars.ReadWrite", "Mail.ReadWrite"}},
		{"send", MailActionPolicy{Send: true}, []string{"User.Read", "Calendars.ReadWrite", "Mail.ReadWrite", "Mail.Send"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got := ScopesForMailPolicy(test.policy)
			if len(got) != len(test.want) {
				t.Fatalf("ScopesForMailPolicy(%+v) = %v, want %v", test.policy, got, test.want)
			}
			for index := range test.want {
				if got[index] != test.want[index] {
					t.Fatalf("ScopesForMailPolicy(%+v) = %v, want %v", test.policy, got, test.want)
				}
			}
		})
	}
}

// TestMailProfileFromConfig verifies legacy feature flags map deterministically.
func TestMailProfileFromConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cfg  config.Config
		want MailProfile
	}{
		{"calendar only", config.Config{}, MailProfileCalendarOnly},
		{"read", config.Config{MailEnabled: true}, MailProfileRead},
		{"manage", config.Config{MailManageEnabled: true}, MailProfileManage},
		{"send", config.Config{MailSendEnabled: true}, MailProfileSend},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := MailProfileFromConfig(tt.cfg); got != tt.want {
				t.Fatalf("MailProfileFromConfig() = %q, want %q", got, tt.want)
			}
		})
	}
}
