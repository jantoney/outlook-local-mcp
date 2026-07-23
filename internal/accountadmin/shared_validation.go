package accountadmin

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/desek/outlook-local-mcp/internal/auth"
	"github.com/desek/outlook-local-mcp/internal/graph"
	"github.com/desek/outlook-local-mcp/internal/resource"
	msgraphcore "github.com/microsoftgraph/msgraph-sdk-go-core"
	"github.com/microsoftgraph/msgraph-sdk-go/models"
	"github.com/microsoftgraph/msgraph-sdk-go/users"
)

// CalendarCandidate is one recipient-view calendar discovered directly from
// /me/calendars. Its ID can be selected for an explicit mounted alias.
type CalendarCandidate struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Owner   string `json:"owner"`
	CanEdit bool   `json:"can_edit"`
}

// RefreshResult contains observational discovery and validation outcomes. It
// never adds, removes, disables, retargets, or changes resource permissions.
type RefreshResult struct {
	Label              string                   `json:"label"`
	CalendarCandidates []CalendarCandidate      `json:"calendar_candidates"`
	DiscoveryError     string                   `json:"discovery_error,omitempty"`
	CalendarAliases    []resource.CalendarAlias `json:"calendar_aliases"`
	MailAliases        []resource.MailAlias     `json:"mail_aliases"`
}

// RefreshSharedResources re-enumerates /me/calendars and directly validates
// every configured shared calendar and mailbox using metadata-only endpoints.
// Validation is observational and fail-closed; configured identities and
// policies are preserved. Discovery failure is reported in the result so users
// can continue with manual entry. Persistence or disconnected-account failures
// are returned as errors.
func (m *Module) RefreshSharedResources(ctx context.Context, label string) (RefreshResult, error) {
	entry, ok := m.registry.Get(label)
	if !ok {
		return RefreshResult{}, fmt.Errorf("account %q not found", label)
	}
	if !entry.Authenticated || entry.Client == nil || entry.ReauthenticationRequired {
		return RefreshResult{}, fmt.Errorf("account %q must be authenticated for its current required scopes", label)
	}
	result := RefreshResult{Label: label}
	candidates, err := discoverCalendarCandidates(ctx, entry, m.cfg.RequestTimeout)
	if err != nil {
		result.DiscoveryError = validationMessage(err)
	} else {
		result.CalendarCandidates = candidates
	}
	now := m.now().UTC()
	calendars := append([]resource.CalendarAlias(nil), entry.CalendarAliases...)
	for index := range calendars {
		if calendars[index].Profile != resource.CalendarProfileOff {
			calendars[index].Validation = resource.Validation{Status: resource.ValidationValidating, Message: "direct Graph metadata validation in progress"}
		}
	}
	mailboxes := append([]resource.MailAlias(nil), entry.MailAliases...)
	for index := range mailboxes {
		if mailboxes[index].Policy != (resource.MailActionPolicy{}) {
			mailboxes[index].Validation = resource.Validation{Status: resource.ValidationValidating, Message: "direct Graph metadata validation in progress"}
		}
	}
	if err := m.persistValidationState(label, calendars, mailboxes, entry.ReauthenticationRequired); err != nil {
		return result, fmt.Errorf("persist validating state: %w", err)
	}
	for index := range calendars {
		calendars[index].Validation = m.validateCalendar(ctx, entry, calendars[index], now)
	}
	for index := range mailboxes {
		mailboxes[index].Validation = m.validateMailbox(ctx, entry, mailboxes[index], now)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.discoveries[label] = append([]CalendarCandidate(nil), result.CalendarCandidates...)
	if err := m.persistValidationState(label, calendars, mailboxes, entry.ReauthenticationRequired); err != nil {
		return result, err
	}
	result.CalendarAliases = calendars
	result.MailAliases = mailboxes
	return result, nil
}

// persistValidationState atomically writes both alias families before
// publishing the same detached values to the runtime registry.
func (m *Module) persistValidationState(label string, calendars []resource.CalendarAlias, mailboxes []resource.MailAlias, reauth bool) error {
	if err := auth.SetAccountSharedResources(m.cfg.AccountsPath, label, calendars, mailboxes, reauth); err != nil {
		return fmt.Errorf("persist shared-resource validation: %w", err)
	}
	return m.registry.Update(label, func(current *auth.AccountEntry) {
		current.CalendarAliases = append([]resource.CalendarAlias(nil), calendars...)
		current.MailAliases = append([]resource.MailAlias(nil), mailboxes...)
	})
}

// ValidateConnectedAccounts starts bounded asynchronous best-effort startup
// validation for every authenticated account whose grant is current. It never
// initiates interactive authentication and returns immediately.
func (m *Module) ValidateConnectedAccounts(ctx context.Context) {
	semaphore := make(chan struct{}, 4)
	for _, account := range m.Accounts() {
		if !account.Authenticated || account.ReauthenticationRequired {
			continue
		}
		label := account.Label
		go func() {
			select {
			case semaphore <- struct{}{}:
				defer func() { <-semaphore }()
			case <-ctx.Done():
				return
			}
			if err := m.validateOwnPermissions(ctx, label); err != nil {
				slog.WarnContext(ctx, "startup Graph permission validation failed", "account", label, "error", validationMessage(err))
			} else {
				slog.InfoContext(ctx, "startup Graph permission validation succeeded", "account", label)
			}
			entry, ok := m.registry.Get(label)
			if ok && (len(entry.CalendarAliases) > 0 || len(entry.MailAliases) > 0) {
				if err := m.validateConfiguredSharedResources(ctx, entry); err != nil {
					slog.WarnContext(ctx, "startup shared-resource validation failed", "account", label, "error", validationMessage(err))
				}
			}
		}()
	}
}

// validateOwnPermissions proves the account's required delegated permissions
// with metadata-only Graph calls. It always checks User.Read through /me with
// only id selected, then probes own calendar or inbox metadata only when those
// resource policies are enabled. It never requests events, messages, or email
// address properties.
func (m *Module) validateOwnPermissions(ctx context.Context, label string) error {
	entry, ok := m.registry.Get(label)
	if !ok || entry.Client == nil {
		return fmt.Errorf("account is not connected")
	}
	requestCtx, cancel := context.WithTimeout(ctx, m.cfg.RequestTimeout)
	defer cancel()
	if _, err := entry.Client.Me().Get(requestCtx, &users.UserItemRequestBuilderGetRequestConfiguration{
		QueryParameters: &users.UserItemRequestBuilderGetQueryParameters{Select: []string{"id"}},
	}); err != nil {
		return fmt.Errorf("validate User.Read: %w", err)
	}
	if entry.CalendarPolicy != auth.CalendarPolicyOff {
		top := int32(1)
		if _, err := entry.Client.Me().Calendars().Get(requestCtx, &users.ItemCalendarsRequestBuilderGetRequestConfiguration{
			QueryParameters: &users.ItemCalendarsRequestBuilderGetQueryParameters{Select: []string{"id"}, Top: &top},
		}); err != nil {
			return fmt.Errorf("validate own calendar permission: %w", err)
		}
	}
	if entry.MailPolicy != (auth.MailActionPolicy{}) {
		if _, err := entry.Client.Me().MailFolders().ByMailFolderId("inbox").Get(requestCtx, nil); err != nil {
			return fmt.Errorf("validate own mail permission: %w", err)
		}
	}
	return nil
}

// validateConfiguredSharedResources directly revalidates persisted aliases
// without calendar discovery. This keeps startup probes free of owner-address
// results while preserving fail-closed routing for configured resources.
func (m *Module) validateConfiguredSharedResources(ctx context.Context, entry *auth.AccountEntry) error {
	now := m.now().UTC()
	calendars := append([]resource.CalendarAlias(nil), entry.CalendarAliases...)
	mailboxes := append([]resource.MailAlias(nil), entry.MailAliases...)
	for index := range calendars {
		calendars[index].Validation = m.validateCalendar(ctx, entry, calendars[index], now)
	}
	for index := range mailboxes {
		mailboxes[index].Validation = m.validateMailbox(ctx, entry, mailboxes[index], now)
	}
	return m.persistValidationState(entry.Label, calendars, mailboxes, entry.ReauthenticationRequired)
}

// validateCalendar performs one bounded metadata request and converts provider
// outcomes into a secret-free validation record.
func (m *Module) validateCalendar(ctx context.Context, entry *auth.AccountEntry, alias resource.CalendarAlias, checkedAt time.Time) resource.Validation {
	if alias.Profile == resource.CalendarProfileOff {
		return resource.Validation{Status: resource.ValidationUnverified, Message: "permission is off", CheckedAt: checkedAt}
	}
	requestCtx, cancel := context.WithTimeout(ctx, m.cfg.RequestTimeout)
	defer cancel()
	var err error
	switch alias.Kind {
	case resource.CalendarKindMounted:
		_, err = entry.Client.Me().Calendars().ByCalendarId(alias.MountedCalendarID).Get(requestCtx, nil)
	case resource.CalendarKindOwnerPrimary:
		_, err = entry.Client.Users().ByUserId(alias.Owner).Calendar().Get(requestCtx, nil)
	default:
		err = fmt.Errorf("unsupported calendar kind")
	}
	return validationFromError(err, checkedAt)
}

// validateMailbox proves owner-view inbox metadata reachability. It does not
// claim Send As, Send on Behalf, write, or folder-level authorization beyond
// the direct request that succeeded.
func (m *Module) validateMailbox(ctx context.Context, entry *auth.AccountEntry, alias resource.MailAlias, checkedAt time.Time) resource.Validation {
	if alias.Policy == (resource.MailActionPolicy{}) {
		return resource.Validation{Status: resource.ValidationUnverified, Message: "all permissions are off", CheckedAt: checkedAt}
	}
	requestCtx, cancel := context.WithTimeout(ctx, m.cfg.RequestTimeout)
	defer cancel()
	_, err := entry.Client.Users().ByUserId(alias.Owner).MailFolders().ByMailFolderId("inbox").Get(requestCtx, nil)
	return validationFromError(err, checkedAt)
}

// discoverCalendarCandidates follows every Graph collection page without
// reading any events. It returns detached metadata-only candidates.
func discoverCalendarCandidates(ctx context.Context, entry *auth.AccountEntry, timeout time.Duration) ([]CalendarCandidate, error) {
	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	response, err := entry.Client.Me().Calendars().Get(requestCtx, nil)
	if err != nil {
		return nil, fmt.Errorf("list calendars: %w", err)
	}
	iterator, err := msgraphcore.NewPageIterator[models.Calendarable](response, entry.Client.GetAdapter(), models.CreateCalendarFromDiscriminatorValue)
	if err != nil {
		return nil, fmt.Errorf("create calendar iterator: %w", err)
	}
	candidates := make([]CalendarCandidate, 0)
	err = iterator.Iterate(requestCtx, func(calendar models.Calendarable) bool {
		owner := ""
		if calendar.GetOwner() != nil {
			owner = graph.SafeStr(calendar.GetOwner().GetAddress())
		}
		candidates = append(candidates, CalendarCandidate{
			ID: graph.SafeStr(calendar.GetId()), Name: graph.SafeStr(calendar.GetName()),
			Owner: owner, CanEdit: graph.SafeBool(calendar.GetCanEdit()),
		})
		return true
	})
	if err != nil {
		return nil, fmt.Errorf("iterate calendars: %w", err)
	}
	return candidates, nil
}

// validationFromError classifies a direct metadata request without exposing
// provider response bodies or resource identifiers.
func validationFromError(err error, checkedAt time.Time) resource.Validation {
	if err == nil {
		return resource.Validation{Status: resource.ValidationAvailable, Message: "direct Graph metadata validation succeeded", CheckedAt: checkedAt}
	}
	status := graph.ExtractHTTPStatus(err)
	if status == 404 {
		return resource.Validation{Status: resource.ValidationUnavailable, Message: "resource was not found", CheckedAt: checkedAt}
	}
	return resource.Validation{Status: resource.ValidationFailed, Message: validationMessage(err), CheckedAt: checkedAt}
}

// validationMessage returns a provider-safe failure summary.
func validationMessage(err error) string {
	status := graph.ExtractHTTPStatus(err)
	if status != 0 {
		return fmt.Sprintf("Graph metadata validation failed (HTTP %d)", status)
	}
	return "Graph metadata validation failed"
}
