package resource

import "fmt"

const (
	// OwnMailboxResourceID is the stable account-bound identity for own mail.
	OwnMailboxResourceID ResourceID = "00000000-0000-4000-8000-000000000001"
	// OwnCalendarResourceID is the stable account-bound identity for own calendars.
	OwnCalendarResourceID ResourceID = "00000000-0000-4000-8000-000000000002"
)

// TargetFamily identifies which separate account-scoped alias namespace a
// request may resolve.
type TargetFamily string

const (
	// TargetFamilyCalendar resolves own or shared calendar targets.
	TargetFamilyCalendar TargetFamily = "calendar"
	// TargetFamilyMail resolves own or shared mailbox targets.
	TargetFamilyMail TargetFamily = "mail"
)

// TargetCapability identifies the exact local action required before route
// construction. Calendar targets support read/manage; mail targets support the
// eight MailCapability string values.
type TargetCapability string

const (
	// TargetCapabilityRead permits a target read.
	TargetCapabilityRead TargetCapability = "read"
	// TargetCapabilityManage permits a calendar mutation.
	TargetCapabilityManage TargetCapability = "manage"
)

// Target is the immutable, authorized resource snapshot carried through one
// request. Its mailbox view alone selects the typed Graph SDK user root.
type Target struct {
	// AccountID binds the target to one signed-in account instance.
	AccountID AccountID
	// ResourceID binds the target to one own or configured resource instance.
	ResourceID ResourceID
	// Alias is the optional configured shared-resource selector.
	Alias string
	// Owner is required only for owner-view targets.
	Owner string
	// Kind is the immutable resource kind.
	Kind ResourceKind
	// View is the immutable Graph identifier namespace.
	View MailboxView
	// MountedCalendarID is required only for mounted recipient-view calendars.
	MountedCalendarID string
	// CalendarProfile is the exact shared-calendar policy when applicable.
	CalendarProfile CalendarProfile
	// MailPolicy is the exact own or shared-mail action matrix when applicable.
	MailPolicy MailActionPolicy
}

// NewOwnTarget creates an account-bound `/me` target for one own-resource
// family. Mail policy is retained only for own mail authorization.
func NewOwnTarget(accountID AccountID, family TargetFamily, mailPolicy MailActionPolicy) (Target, error) {
	return NewOwnTargetWithCalendarPolicy(accountID, family, CalendarProfileManage, mailPolicy)
}

// NewOwnTargetWithCalendarPolicy creates an account-bound `/me` target with an
// explicit own-calendar policy. CalendarPolicyOff denies all calendar actions,
// while read and manage use the same capability vocabulary as shared targets.
// Mail authorization continues to use the independent mail action matrix.
func NewOwnTargetWithCalendarPolicy(accountID AccountID, family TargetFamily, calendarPolicy CalendarProfile, mailPolicy MailActionPolicy) (Target, error) {
	target := Target{AccountID: accountID, View: MailboxViewRecipient}
	switch family {
	case TargetFamilyCalendar:
		target.ResourceID = OwnCalendarResourceID
		target.Kind = ResourceKindOwnCalendar
		target.CalendarProfile = calendarPolicy
	case TargetFamilyMail:
		target.ResourceID = OwnMailboxResourceID
		target.Kind = ResourceKindOwnMailbox
		target.MailPolicy = mailPolicy
	default:
		return Target{}, fmt.Errorf("invalid target family %q", family)
	}
	if err := target.Validate(); err != nil {
		return Target{}, err
	}
	return target, nil
}

// TargetFromCalendarAlias copies one validated configured calendar identity
// into an immutable request target. It does not decide compatibility.
func TargetFromCalendarAlias(accountID AccountID, alias CalendarAlias) (Target, error) {
	if err := alias.Validate(); err != nil {
		return Target{}, err
	}
	target := Target{
		AccountID: accountID, ResourceID: alias.ResourceID, Alias: alias.Alias,
		Owner: alias.Owner, Kind: ResourceKind(alias.Kind), View: alias.View,
		MountedCalendarID: alias.MountedCalendarID, CalendarProfile: alias.Profile,
	}
	if err := target.Validate(); err != nil {
		return Target{}, err
	}
	return target, nil
}

// TargetFromMailAlias copies one validated configured mailbox identity and
// exact action matrix into an immutable request target.
func TargetFromMailAlias(accountID AccountID, alias MailAlias) (Target, error) {
	if err := alias.Validate(); err != nil {
		return Target{}, err
	}
	target := Target{
		AccountID: accountID, ResourceID: alias.ResourceID, Alias: alias.Alias,
		Owner: alias.Owner, Kind: alias.Kind, View: alias.View, MailPolicy: alias.Policy,
	}
	if err := target.Validate(); err != nil {
		return Target{}, err
	}
	return target, nil
}

// Validate rejects incomplete targets and every resource-kind/mailbox-view
// mismatch before a typed Graph request builder can be selected.
func (t Target) Validate() error {
	if err := t.AccountID.Validate(); err != nil {
		return err
	}
	if err := t.ResourceID.Validate(); err != nil {
		return err
	}
	if err := validateResourceView(t.Kind, t.View); err != nil {
		return err
	}
	switch t.Kind {
	case ResourceKindMailbox, ResourceKindOwnerPrimaryCalendar:
		if t.Owner == "" {
			return fmt.Errorf("owner-view target requires an owner locator")
		}
		if t.MountedCalendarID != "" {
			return fmt.Errorf("owner-view target cannot carry a mounted calendar ID")
		}
	case ResourceKindMountedCalendar:
		if t.Owner == "" || t.MountedCalendarID == "" {
			return fmt.Errorf("mounted calendar target requires owner provenance and mounted calendar ID")
		}
	case ResourceKindOwnMailbox, ResourceKindOwnCalendar:
		if t.Owner != "" || t.Alias != "" || t.MountedCalendarID != "" {
			return fmt.Errorf("own target cannot carry shared-resource routing fields")
		}
	}
	return nil
}

// Family returns the target's separate calendar or mail namespace. Invalid
// resource kinds return an error and no fallback family.
func (t Target) Family() (TargetFamily, error) {
	switch t.Kind {
	case ResourceKindOwnCalendar, ResourceKindOwnerPrimaryCalendar, ResourceKindMountedCalendar:
		return TargetFamilyCalendar, nil
	case ResourceKindOwnMailbox, ResourceKindMailbox:
		return TargetFamilyMail, nil
	default:
		return "", fmt.Errorf("invalid resource kind %q", t.Kind)
	}
}

// Allows reports whether the target snapshot permits capability. It fails
// closed for cross-family, malformed, or unsupported capabilities.
func (t Target) Allows(capability TargetCapability) bool {
	switch t.Kind {
	case ResourceKindOwnCalendar:
		return (capability == TargetCapabilityRead && (t.CalendarProfile == CalendarProfileRead || t.CalendarProfile == CalendarProfileManage)) ||
			(capability == TargetCapabilityManage && t.CalendarProfile == CalendarProfileManage)
	case ResourceKindOwnerPrimaryCalendar:
		return capability == TargetCapabilityRead && t.CalendarProfile == CalendarProfileRead
	case ResourceKindMountedCalendar:
		return (capability == TargetCapabilityRead && (t.CalendarProfile == CalendarProfileRead || t.CalendarProfile == CalendarProfileManage)) ||
			(capability == TargetCapabilityManage && t.CalendarProfile == CalendarProfileManage)
	case ResourceKindOwnMailbox, ResourceKindMailbox:
		return t.MailPolicy.Allows(MailCapability(capability))
	default:
		return false
	}
}
