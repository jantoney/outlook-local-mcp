package auth

import (
	"fmt"

	"github.com/desek/outlook-local-mcp/internal/config"
)

// MailProfile is an ordered per-account mail capability profile. Higher
// profiles include every capability of lower profiles.
type MailProfile uint8

const (
	// MailProfileCalendarOnly grants no mail capability.
	MailProfileCalendarOnly MailProfile = iota
	// MailProfileRead grants read-only mail capability.
	MailProfileRead
	// MailProfileManage grants draft and attachment management capability.
	MailProfileManage
	// MailProfileSend grants confirmed sending of existing drafts.
	MailProfileSend
)

// ParseMailProfile converts a persisted or user-provided profile name to its
// typed value. It returns an error for unknown names.
func ParseMailProfile(value string) (MailProfile, error) {
	switch value {
	case "calendar_only":
		return MailProfileCalendarOnly, nil
	case "mail_read":
		return MailProfileRead, nil
	case "mail_manage":
		return MailProfileManage, nil
	case "mail_send":
		return MailProfileSend, nil
	default:
		return MailProfileCalendarOnly, fmt.Errorf("invalid mail profile %q: expected calendar_only, mail_read, mail_manage, or mail_send", value)
	}
}

// String returns the stable configuration and persistence name for the
// profile.
func (p MailProfile) String() string {
	switch p {
	case MailProfileRead:
		return "mail_read"
	case MailProfileManage:
		return "mail_manage"
	case MailProfileSend:
		return "mail_send"
	default:
		return "calendar_only"
	}
}

// Allows reports whether this profile contains the required profile's
// capabilities.
func (p MailProfile) Allows(required MailProfile) bool {
	return p >= required
}

// MailProfileFromConfig derives the backward-compatible profile used by the
// implicit account and legacy account records.
func MailProfileFromConfig(cfg config.Config) MailProfile {
	switch {
	case cfg.MailSendEnabled:
		return MailProfileSend
	case cfg.MailManageEnabled:
		return MailProfileManage
	case cfg.MailEnabled:
		return MailProfileRead
	default:
		return MailProfileCalendarOnly
	}
}
