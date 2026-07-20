package resource

import (
	"fmt"
	"regexp"
)

var aliasPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

// CalendarKind identifies the immutable Graph mailbox-view contract of a
// configured shared calendar.
type CalendarKind string

const (
	// CalendarKindOwnerPrimary is the owner's primary calendar in owner view.
	CalendarKindOwnerPrimary CalendarKind = "owner_primary_calendar"
	// CalendarKindMounted is a selected calendar mounted in recipient view.
	CalendarKindMounted CalendarKind = "mounted_calendar"
)

// CalendarProfile is the exact local capability level for one calendar alias.
type CalendarProfile string

const (
	// CalendarProfileOff disables every operation for the alias.
	CalendarProfileOff CalendarProfile = "off"
	// CalendarProfileRead permits supported shared calendar reads.
	CalendarProfileRead CalendarProfile = "read"
	// CalendarProfileManage permits supported mounted-calendar mutations.
	CalendarProfileManage CalendarProfile = "manage"
)

// CalendarAlias is one persisted account-scoped shared-calendar selector. Its
// owner, kind, view, mounted ID, and resource ID are immutable after creation;
// only Alias and Profile may change through explicit lifecycle operations.
type CalendarAlias struct {
	// ResourceID is the immutable local provenance identity.
	ResourceID ResourceID `json:"resource_id"`
	// Alias is the mutable human-facing account-scoped selector.
	Alias string `json:"alias"`
	// Owner is the Graph-compatible owner locator configured by the user.
	Owner string `json:"owner"`
	// Kind fixes the calendar's owner-primary or mounted identity model.
	Kind CalendarKind `json:"kind"`
	// View fixes the Graph mailbox view that owns returned identifiers.
	View MailboxView `json:"view"`
	// MountedCalendarID is the explicitly selected recipient-view calendar ID.
	MountedCalendarID string `json:"mounted_calendar_id,omitempty"`
	// Profile is the target-local off, read, or manage authorization level.
	Profile CalendarProfile `json:"profile"`
}

// NewOwnerPrimaryCalendar creates and validates an organizational owner-view
// primary-calendar alias. Owner-primary manage is rejected locally.
func NewOwnerPrimaryCalendar(id ResourceID, alias, owner string, profile CalendarProfile) (CalendarAlias, error) {
	calendar := CalendarAlias{
		ResourceID: id, Alias: alias, Owner: owner, Kind: CalendarKindOwnerPrimary,
		View: MailboxViewOwner, Profile: profile,
	}
	if err := calendar.Validate(); err != nil {
		return CalendarAlias{}, err
	}
	return calendar, nil
}

// NewMountedCalendar creates and validates a recipient-view mounted-calendar
// alias using the exact calendar ID selected during discovery.
func NewMountedCalendar(id ResourceID, alias, owner, mountedID string, profile CalendarProfile) (CalendarAlias, error) {
	calendar := CalendarAlias{
		ResourceID: id, Alias: alias, Owner: owner, Kind: CalendarKindMounted,
		View: MailboxViewRecipient, MountedCalendarID: mountedID, Profile: profile,
	}
	if err := calendar.Validate(); err != nil {
		return CalendarAlias{}, err
	}
	return calendar, nil
}

// Validate verifies the alias identity, immutable kind/view combination,
// owner locator, mounted ID requirements, and profile compatibility. It makes
// no Graph calls or mutations.
func (a CalendarAlias) Validate() error {
	if err := a.ResourceID.Validate(); err != nil {
		return err
	}
	if !aliasPattern.MatchString(a.Alias) {
		return fmt.Errorf("invalid calendar alias %q: must match %s", a.Alias, aliasPattern.String())
	}
	if a.Owner == "" {
		return fmt.Errorf("calendar alias owner must not be empty")
	}
	if a.Profile != CalendarProfileOff && a.Profile != CalendarProfileRead && a.Profile != CalendarProfileManage {
		return fmt.Errorf("invalid calendar profile %q", a.Profile)
	}
	switch a.Kind {
	case CalendarKindOwnerPrimary:
		if a.View != MailboxViewOwner || a.MountedCalendarID != "" {
			return fmt.Errorf("owner-primary calendar requires owner view without a mounted calendar ID")
		}
		if a.Profile == CalendarProfileManage {
			return fmt.Errorf("owner-primary calendar aliases are read-only")
		}
	case CalendarKindMounted:
		if a.View != MailboxViewRecipient || a.MountedCalendarID == "" {
			return fmt.Errorf("mounted calendar requires recipient view and an explicit mounted calendar ID")
		}
	default:
		return fmt.Errorf("invalid calendar kind %q", a.Kind)
	}
	return nil
}

// Rename returns a copy with a new display alias while preserving immutable
// identity and routing fields. It returns an error for an invalid new alias.
func (a CalendarAlias) Rename(alias string) (CalendarAlias, error) {
	a.Alias = alias
	if err := a.Validate(); err != nil {
		return CalendarAlias{}, err
	}
	return a, nil
}

// WithProfile returns a copy with a new target-local profile while preserving
// immutable identity and routing fields. Incompatible profiles return an error.
func (a CalendarAlias) WithProfile(profile CalendarProfile) (CalendarAlias, error) {
	a.Profile = profile
	if err := a.Validate(); err != nil {
		return CalendarAlias{}, err
	}
	return a, nil
}

// ReselectMounted returns a copy with a newly human-selected mounted calendar
// ID while preserving resource identity, owner, kind, view, alias, and policy.
// It rejects owner-primary aliases and empty IDs.
func (a CalendarAlias) ReselectMounted(mountedID string) (CalendarAlias, error) {
	if a.Kind != CalendarKindMounted {
		return CalendarAlias{}, fmt.Errorf("only mounted calendar aliases can be reselected")
	}
	a.MountedCalendarID = mountedID
	if err := a.Validate(); err != nil {
		return CalendarAlias{}, err
	}
	return a, nil
}

// ValidateCalendarCompatibility enforces token-context restrictions before
// Graph traffic. Owner-view primary calendars require an organizational token;
// mounted manage also requires organizational context. Mounted read and off
// remain compatible with personal, organizational, or unknown contexts.
func ValidateCalendarCompatibility(alias CalendarAlias, tokenTenantContext string) error {
	if err := alias.Validate(); err != nil {
		return err
	}
	if alias.Kind == CalendarKindOwnerPrimary && tokenTenantContext != "organizational" {
		return fmt.Errorf("owner-primary calendar aliases require an organizational token context")
	}
	if alias.Profile == CalendarProfileManage && tokenTenantContext != "organizational" {
		return fmt.Errorf("shared calendar manage requires an organizational token context")
	}
	return nil
}
