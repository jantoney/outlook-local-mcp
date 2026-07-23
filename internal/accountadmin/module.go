package accountadmin

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/desek/outlook-local-mcp/internal/auth"
	"github.com/desek/outlook-local-mcp/internal/config"
	"github.com/desek/outlook-local-mcp/internal/resource"
	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"
)

// Module coordinates persisted account configuration and runtime registry
// state. Its mutex serializes mutations across HTTP and MCP transports.
type Module struct {
	cfg             config.Config
	registry        *auth.AccountRegistry
	mu              sync.Mutex
	rootCtx         context.Context
	sessions        map[string]*authSession
	sessionByLabel  map[string]string
	discoveries     map[string][]CalendarCandidate
	credentialSetup auth.CredentialFactory
	clientFactory   func(azcore.TokenCredential, []string) (*msgraphsdk.GraphServiceClient, error)
	now             func() time.Time
}

// New constructs an administration module bound to cfg and registry. rootCtx
// controls authentication-session lifetime; cancelling it cancels every active
// attempt. The function performs no I/O.
func New(rootCtx context.Context, cfg config.Config, registry *auth.AccountRegistry) *Module {
	if rootCtx == nil {
		rootCtx = context.Background()
	}
	return &Module{
		cfg:             cfg,
		registry:        registry,
		rootCtx:         rootCtx,
		sessions:        make(map[string]*authSession),
		sessionByLabel:  make(map[string]string),
		discoveries:     make(map[string][]CalendarCandidate),
		credentialSetup: auth.SetupCredentialForAccount,
		clientFactory: func(credential azcore.TokenCredential, scopes []string) (*msgraphsdk.GraphServiceClient, error) {
			return auth.NewDefaultGraphClientFactory(scopes)(credential)
		},
		now: time.Now,
	}
}

// Accounts returns sorted, detached secret-free snapshots of all registered
// accounts. It performs no network or filesystem I/O.
func (m *Module) Accounts() []Account {
	entries := m.registry.List()
	result := make([]Account, 0, len(entries))
	for _, entry := range entries {
		view := accountView(entry)
		m.mu.Lock()
		view.CalendarCandidates = append([]CalendarCandidate(nil), m.discoveries[entry.Label]...)
		m.mu.Unlock()
		result = append(result, view)
	}
	return result
}

// Account returns a secret-free snapshot by label. It returns false when the
// label is unknown and performs no I/O.
func (m *Module) Account(label string) (Account, bool) {
	entry, ok := m.registry.Get(label)
	if !ok {
		return Account{}, false
	}
	view := accountView(entry)
	m.mu.Lock()
	view.CalendarCandidates = append([]CalendarCandidate(nil), m.discoveries[label]...)
	m.mu.Unlock()
	return view, true
}

// CreateAccount persists and registers a new disconnected account with the
// requested explicit policies. It applies configured identity defaults for
// empty fields, generates an immutable local ID, and marks authentication as
// required. In read-only mode, only an all-off initial policy is accepted.
func (m *Module) CreateAccount(request CreateRequest) (Account, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.cfg.ReadOnly && (request.CalendarPolicy != auth.CalendarPolicyOff || request.MailPolicy != (auth.MailActionPolicy{})) {
		return Account{}, fmt.Errorf("read-only mode permits account lifecycle but not enabling permissions")
	}
	if request.CalendarPolicy == "" {
		request.CalendarPolicy = auth.CalendarPolicyOff
	}
	if _, err := auth.ParseCalendarPolicy(string(request.CalendarPolicy)); err != nil {
		return Account{}, err
	}
	if request.ClientID == "" {
		request.ClientID = m.cfg.ClientID
	}
	if request.TenantID == "" {
		request.TenantID = m.cfg.TenantID
	}
	if request.AuthMethod == "" {
		request.AuthMethod = m.cfg.AuthMethod
	}
	if err := validateAuthMethod(request.AuthMethod); err != nil {
		return Account{}, err
	}
	if _, exists := m.registry.Get(request.Label); exists {
		return Account{}, fmt.Errorf("account %q already exists", request.Label)
	}
	accountID, err := auth.NewAccountID()
	if err != nil {
		return Account{}, fmt.Errorf("create account identity: %w", err)
	}
	entry := &auth.AccountEntry{
		AccountID: accountID, Label: request.Label, ClientID: request.ClientID,
		TenantID: request.TenantID, AuthMethod: request.AuthMethod,
		CalendarPolicy: request.CalendarPolicy, MailPolicy: request.MailPolicy,
		ReauthenticationRequired: true,
	}
	if err := m.registry.Add(entry); err != nil {
		return Account{}, err
	}
	persist := auth.AccountConfig{
		AccountID: accountID, Label: request.Label, ClientID: request.ClientID,
		TenantID: request.TenantID, AuthMethod: request.AuthMethod,
		CalendarPolicy: &request.CalendarPolicy, MailPolicy: &request.MailPolicy,
		ReauthenticationRequired: true,
	}
	if err := auth.AddAccountConfig(m.cfg.AccountsPath, persist); err != nil {
		_ = m.registry.Remove(request.Label)
		return Account{}, fmt.Errorf("persist account %q: %w", request.Label, err)
	}
	return accountView(entry), nil
}

// SetPermissions persists an account's exact own-calendar and own-mail policy
// before mutating runtime state. Same-scope changes take effect immediately.
// Scope changes disconnect the account, clear active handles and local auth
// material, and durably mark re-authentication required.
func (m *Module) SetPermissions(label string, update PermissionUpdate) (Account, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cfg.ReadOnly {
		return Account{}, fmt.Errorf("permission changes are disabled in read-only mode")
	}
	if _, err := auth.ParseCalendarPolicy(string(update.Calendar)); err != nil {
		return Account{}, err
	}
	entry, ok := m.registry.Get(label)
	if !ok {
		return Account{}, fmt.Errorf("account %q not found", label)
	}
	oldScopes := auth.ScopesForAccountEntry(entry)
	newScopes := auth.RequiredScopes(update.Calendar, update.Mail, entry.CalendarAliases, entry.MailAliases)
	scopesChanged := !auth.OAuthScopeSetEqual(oldScopes, newScopes)
	reauth := entry.ReauthenticationRequired || scopesChanged
	if err := auth.SetAccountPermissions(m.cfg.AccountsPath, label, update.Calendar, update.Mail, reauth); err != nil {
		return Account{}, fmt.Errorf("persist account %q permissions: %w", label, err)
	}
	if err := m.registry.Update(label, func(current *auth.AccountEntry) {
		current.CalendarPolicy = update.Calendar
		current.MailPolicy = update.Mail
		current.ReauthenticationRequired = reauth
		if scopesChanged {
			current.Authenticated = false
			current.Scopes = nil
			current.Client = nil
			current.Credential = nil
			current.Authenticator = nil
		}
	}); err != nil {
		return Account{}, fmt.Errorf("update account %q runtime permissions: %w", label, err)
	}
	if scopesChanged {
		if err := clearAuthentication(entry); err != nil {
			return Account{}, fmt.Errorf("permissions saved and account denied, but authentication cleanup failed: %w", err)
		}
	}
	updated, _ := m.registry.Get(label)
	return accountView(updated), nil
}

// Logout disconnects an account and clears its cached authentication while
// preserving identity and policies. Repeated logout is safe. It returns an
// error only for an unknown account or cleanup failure.
func (m *Module) Logout(label string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	entry, ok := m.registry.Get(label)
	if !ok {
		return fmt.Errorf("account %q not found", label)
	}
	if err := m.registry.Update(label, func(current *auth.AccountEntry) {
		current.Authenticated = false
		current.Scopes = nil
		current.Client = nil
		current.Credential = nil
		current.Authenticator = nil
	}); err != nil {
		return err
	}
	return clearAuthentication(entry)
}

// Remove deletes an account from persistence and runtime and best-effort clears
// local authentication. The mutation is serialized across transports. It
// returns an error when the label is unknown or persistence fails.
func (m *Module) Remove(label string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	entry, ok := m.registry.Get(label)
	if !ok {
		return fmt.Errorf("account %q not found", label)
	}
	if err := auth.RemoveAccountConfig(m.cfg.AccountsPath, label); err != nil {
		return fmt.Errorf("remove persisted account %q: %w", label, err)
	}
	if err := m.registry.Remove(label); err != nil {
		return err
	}
	return clearAuthentication(entry)
}

// accountView converts one registry snapshot into a transport-safe view.
func accountView(entry *auth.AccountEntry) Account {
	return Account{
		AccountID: entry.AccountID, Label: entry.Label, UPN: entry.Email,
		ClientID: entry.ClientID, TenantID: entry.TenantID, AuthMethod: entry.AuthMethod,
		CalendarPolicy: entry.CalendarPolicy, MailPolicy: entry.MailPolicy,
		CalendarAliases:          append([]resource.CalendarAlias(nil), entry.CalendarAliases...),
		MailAliases:              append([]resource.MailAlias(nil), entry.MailAliases...),
		Authenticated:            entry.Authenticated,
		ReauthenticationRequired: entry.ReauthenticationRequired,
		ActiveScopes:             append([]string(nil), entry.Scopes...),
		RequiredScopes:           auth.ScopesForAccountEntry(entry),
	}
}

// clearAuthentication removes the token cache and auth record for entry. It
// joins all cleanup failures so callers can report partial local cleanup.
func clearAuthentication(entry *auth.AccountEntry) error {
	var errs []error
	if entry.CacheName != "" {
		if err := auth.ClearTokenCache(entry.CacheName); err != nil {
			errs = append(errs, fmt.Errorf("token cache: %w", err))
		}
	}
	if entry.AuthRecordPath != "" {
		if err := os.Remove(entry.AuthRecordPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, fmt.Errorf("authentication record: %w", err))
		}
	}
	return errors.Join(errs...)
}

// validateAuthMethod returns an error unless value names a supported account
// authentication method. It performs no side effects.
func validateAuthMethod(value string) error {
	switch strings.ToLower(value) {
	case "browser", "device_code", "auth_code":
		return nil
	default:
		return fmt.Errorf("invalid auth method %q", value)
	}
}

// authRecordDir returns the configured directory for account auth records.
func (m *Module) authRecordDir() string { return filepath.Dir(m.cfg.AuthRecordPath) }
