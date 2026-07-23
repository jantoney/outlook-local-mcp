package accountadmin

import (
	"fmt"

	"github.com/desek/outlook-local-mcp/internal/auth"
	"github.com/desek/outlook-local-mcp/internal/resource"
)

// AddCalendar configures one manual shared calendar. Mounted IDs are retained
// exactly as entered and must pass direct Graph validation before routing is
// available. Owner-primary manage is rejected by the resource constructor.
func (m *Module) AddCalendar(label, alias, owner string, kind resource.CalendarKind, mountedID string, profile resource.CalendarProfile) (Account, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cfg.ReadOnly {
		return Account{}, fmt.Errorf("shared-resource changes are disabled in read-only mode")
	}
	entry, ok := m.registry.Get(label)
	if !ok {
		return Account{}, fmt.Errorf("account %q not found", label)
	}
	if sharedAliasExists(entry, alias) {
		return Account{}, fmt.Errorf("shared-resource alias %q already exists", alias)
	}
	id, err := resource.NewResourceID()
	if err != nil {
		return Account{}, err
	}
	var calendar resource.CalendarAlias
	switch kind {
	case resource.CalendarKindMounted:
		calendar, err = resource.NewMountedCalendar(id, alias, owner, mountedID, profile)
	case resource.CalendarKindOwnerPrimary:
		calendar, err = resource.NewOwnerPrimaryCalendar(id, alias, owner, profile)
	default:
		err = fmt.Errorf("invalid calendar kind %q", kind)
	}
	if err != nil {
		return Account{}, err
	}
	if profile == resource.CalendarProfileOff {
		calendar.Validation = resource.Validation{Status: resource.ValidationUnverified, Message: "permission is off"}
	} else {
		calendar.Validation = resource.Validation{Status: resource.ValidationReauthenticationRequired, Message: "authenticate and validate before use"}
	}
	calendars := append(append([]resource.CalendarAlias(nil), entry.CalendarAliases...), calendar)
	return m.applySharedResourcesLocked(entry, calendars, entry.MailAliases)
}

// AddMailbox configures a manual shared mailbox with every action disabled and
// unverified. Permissions must be enabled explicitly, followed by authentication
// when scopes change and direct Graph validation before use.
func (m *Module) AddMailbox(label, alias, owner string) (Account, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cfg.ReadOnly {
		return Account{}, fmt.Errorf("shared-resource changes are disabled in read-only mode")
	}
	entry, ok := m.registry.Get(label)
	if !ok {
		return Account{}, fmt.Errorf("account %q not found", label)
	}
	if sharedAliasExists(entry, alias) {
		return Account{}, fmt.Errorf("shared-resource alias %q already exists", alias)
	}
	id, err := resource.NewResourceID()
	if err != nil {
		return Account{}, err
	}
	mailbox, err := resource.NewMailAlias(id, alias, owner)
	if err != nil {
		return Account{}, err
	}
	mailbox.Validation = resource.Validation{Status: resource.ValidationUnverified, Message: "all permissions are off"}
	mailboxes := append(append([]resource.MailAlias(nil), entry.MailAliases...), mailbox)
	return m.applySharedResourcesLocked(entry, entry.CalendarAliases, mailboxes)
}

// sharedAliasExists reports whether alias is already used by either shared
// resource family. One account-wide namespace keeps adapter routing unambiguous.
func sharedAliasExists(entry *auth.AccountEntry, alias string) bool {
	for _, calendar := range entry.CalendarAliases {
		if calendar.Alias == alias {
			return true
		}
	}
	for _, mailbox := range entry.MailAliases {
		if mailbox.Alias == alias {
			return true
		}
	}
	return false
}

// SetCalendarProfile changes only one alias's local capability. The immutable
// owner, kind, view, mounted ID, and resource identity are preserved.
func (m *Module) SetCalendarProfile(label, alias string, profile resource.CalendarProfile) (Account, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cfg.ReadOnly {
		return Account{}, fmt.Errorf("shared-resource changes are disabled in read-only mode")
	}
	entry, ok := m.registry.Get(label)
	if !ok {
		return Account{}, fmt.Errorf("account %q not found", label)
	}
	calendars := append([]resource.CalendarAlias(nil), entry.CalendarAliases...)
	found := false
	for index := range calendars {
		if calendars[index].Alias != alias {
			continue
		}
		updated, err := calendars[index].WithProfile(profile)
		if err != nil {
			return Account{}, err
		}
		updated.Validation = resource.Validation{Status: resource.ValidationReauthenticationRequired, Message: "authenticate and validate before use"}
		if profile == resource.CalendarProfileOff {
			updated.Validation = resource.Validation{Status: resource.ValidationUnverified, Message: "permission is off"}
		}
		calendars[index] = updated
		found = true
		break
	}
	if !found {
		return Account{}, fmt.Errorf("calendar alias %q not found", alias)
	}
	return m.applySharedResourcesLocked(entry, calendars, entry.MailAliases)
}

// SetMailboxPolicy replaces one alias's independent action matrix while
// preserving immutable routing identity.
func (m *Module) SetMailboxPolicy(label, alias string, policy auth.MailActionPolicy) (Account, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cfg.ReadOnly {
		return Account{}, fmt.Errorf("shared-resource changes are disabled in read-only mode")
	}
	entry, ok := m.registry.Get(label)
	if !ok {
		return Account{}, fmt.Errorf("account %q not found", label)
	}
	mailboxes := append([]resource.MailAlias(nil), entry.MailAliases...)
	found := false
	for index := range mailboxes {
		if mailboxes[index].Alias != alias {
			continue
		}
		mailboxes[index] = mailboxes[index].WithPolicy(policy)
		mailboxes[index].Validation = resource.Validation{Status: resource.ValidationReauthenticationRequired, Message: "authenticate and validate before use"}
		if policy == (auth.MailActionPolicy{}) {
			mailboxes[index].Validation = resource.Validation{Status: resource.ValidationUnverified, Message: "all permissions are off"}
		}
		found = true
		break
	}
	if !found {
		return Account{}, fmt.Errorf("mail alias %q not found", alias)
	}
	return m.applySharedResourcesLocked(entry, entry.CalendarAliases, mailboxes)
}

// RemoveSharedResource removes exactly one alias after explicit adapter-level
// confirmation. family must be calendar or mail.
func (m *Module) RemoveSharedResource(label, family, alias string) (Account, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cfg.ReadOnly {
		return Account{}, fmt.Errorf("shared-resource changes are disabled in read-only mode")
	}
	entry, ok := m.registry.Get(label)
	if !ok {
		return Account{}, fmt.Errorf("account %q not found", label)
	}
	calendars := append([]resource.CalendarAlias(nil), entry.CalendarAliases...)
	mailboxes := append([]resource.MailAlias(nil), entry.MailAliases...)
	found := false
	switch family {
	case "calendar":
		filtered := calendars[:0]
		for _, current := range calendars {
			if current.Alias == alias {
				found = true
				continue
			}
			filtered = append(filtered, current)
		}
		calendars = filtered
	case "mail":
		filtered := mailboxes[:0]
		for _, current := range mailboxes {
			if current.Alias == alias {
				found = true
				continue
			}
			filtered = append(filtered, current)
		}
		mailboxes = filtered
	default:
		return Account{}, fmt.Errorf("invalid shared-resource family %q", family)
	}
	if !found {
		return Account{}, fmt.Errorf("%s alias %q not found", family, alias)
	}
	return m.applySharedResourcesLocked(entry, calendars, mailboxes)
}

// applySharedResourcesLocked persists both alias families and transitions auth
// state when their scope union changes. The caller must hold m.mu.
func (m *Module) applySharedResourcesLocked(entry *auth.AccountEntry, calendars []resource.CalendarAlias, mailboxes []resource.MailAlias) (Account, error) {
	oldScopes := auth.ScopesForAccountEntry(entry)
	newScopes := auth.RequiredScopes(entry.CalendarPolicy, entry.MailPolicy, calendars, mailboxes)
	scopesChanged := !auth.OAuthScopeSetEqual(oldScopes, newScopes)
	reauth := entry.ReauthenticationRequired || scopesChanged
	if err := auth.SetAccountSharedResources(m.cfg.AccountsPath, entry.Label, calendars, mailboxes, reauth); err != nil {
		return Account{}, err
	}
	if err := m.registry.Update(entry.Label, func(current *auth.AccountEntry) {
		current.CalendarAliases = append([]resource.CalendarAlias(nil), calendars...)
		current.MailAliases = append([]resource.MailAlias(nil), mailboxes...)
		current.ReauthenticationRequired = reauth
		if scopesChanged {
			current.Authenticated = false
			current.Scopes = nil
			current.Client = nil
			current.Credential = nil
			current.Authenticator = nil
		}
	}); err != nil {
		return Account{}, err
	}
	if scopesChanged {
		if err := clearAuthentication(entry); err != nil {
			return Account{}, fmt.Errorf("shared resources saved and account denied, but authentication cleanup failed: %w", err)
		}
	}
	updated, _ := m.registry.Get(entry.Label)
	return accountView(updated), nil
}
