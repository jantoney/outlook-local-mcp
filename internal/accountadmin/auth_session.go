package accountadmin

import (
	"context"
	"fmt"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/desek/outlook-local-mcp/internal/auth"
	"github.com/google/uuid"
)

const authSessionLifetime = 15 * time.Minute

// authSession stores process-bound authentication collaborators. It is never
// returned directly because it contains credentials and cancellation handles.
type authSession struct {
	view           AuthSession
	credential     azcore.TokenCredential
	authenticator  auth.Authenticator
	authRecordPath string
	cacheName      string
	scopes         []string
	cancel         context.CancelFunc
}

// StartAuthentication starts or resumes one process-bound authentication
// attempt for label. Browser and device-code methods continue asynchronously;
// auth_code returns an authorization URL and waits for CompleteAuthentication.
// The returned snapshot never contains tokens or PKCE verifier material.
func (m *Module) StartAuthentication(label string) (AuthSession, error) {
	m.mu.Lock()
	m.expireSessionsLocked()
	if id, exists := m.sessionByLabel[label]; exists {
		if session := m.sessions[id]; session != nil && activeSession(session.view.State) {
			view := session.view
			m.mu.Unlock()
			return view, nil
		}
	}
	entry, ok := m.registry.Get(label)
	if !ok {
		m.mu.Unlock()
		return AuthSession{}, fmt.Errorf("account %q not found", label)
	}
	scopes := auth.ScopesForAccountEntry(entry)
	credential, authenticator, authRecordPath, cacheName, err := m.credentialSetup(
		entry.Label, entry.ClientID, entry.TenantID, entry.AuthMethod,
		m.cfg.CacheName, m.authRecordDir(), m.cfg.TokenStorage,
	)
	if err != nil {
		m.mu.Unlock()
		return AuthSession{}, fmt.Errorf("set up account %q credential: %w", label, err)
	}
	created := m.now()
	ctx, cancel := context.WithDeadline(m.rootCtx, created.Add(authSessionLifetime))
	session := &authSession{
		view: AuthSession{
			ID: uuid.NewString(), Label: label, State: AuthSessionStarting,
			CreatedAt: created, ExpiresAt: created.Add(authSessionLifetime),
		},
		credential: credential, authenticator: authenticator,
		authRecordPath: authRecordPath, cacheName: cacheName,
		scopes: append([]string(nil), scopes...), cancel: cancel,
	}
	m.sessions[session.view.ID] = session
	m.sessionByLabel[label] = session.view.ID

	if entry.AuthMethod == "auth_code" {
		flow, supports := authenticator.(auth.AuthCodeFlow)
		if !supports {
			delete(m.sessions, session.view.ID)
			delete(m.sessionByLabel, label)
			cancel()
			m.mu.Unlock()
			return AuthSession{}, fmt.Errorf("account %q credential does not support auth_code", label)
		}
		authURL, urlErr := flow.AuthCodeURL(ctx, scopes)
		if urlErr != nil {
			session.view.State = AuthSessionFailed
			session.view.Error = urlErr.Error()
			view := session.view
			m.mu.Unlock()
			return view, nil
		}
		session.view.State = AuthSessionAwaitingCode
		session.view.AuthURL = authURL
		session.view.Prompt = "Open the Microsoft sign-in page, then paste the full redirect URL."
		view := session.view
		m.mu.Unlock()
		return view, nil
	}

	session.view.State = AuthSessionWaiting
	if entry.AuthMethod == "browser" {
		session.view.Prompt = "Complete Microsoft sign-in in the browser window."
	} else {
		session.view.Prompt = "Requesting a Microsoft device code…"
	}
	view := session.view
	m.mu.Unlock()
	go m.runInteractiveAuthentication(ctx, session.view.ID, entry.AuthMethod)
	return view, nil
}

// Session returns the current secret-free authentication session snapshot. It
// expires overdue sessions before lookup and returns false for unknown IDs.
func (m *Module) Session(id string) (AuthSession, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.expireSessionsLocked()
	session, ok := m.sessions[id]
	if !ok {
		return AuthSession{}, false
	}
	return session.view, true
}

// CompleteAuthentication exchanges an auth_code redirect URL and finalizes the
// account's current Graph client. It returns an error for an unknown, expired,
// or wrong-state session, malformed redirect, provider failure, or persistence
// failure. Tokens and authorization codes are never retained in the view.
func (m *Module) CompleteAuthentication(id, redirectURL string) (AuthSession, error) {
	m.mu.Lock()
	m.expireSessionsLocked()
	session, ok := m.sessions[id]
	if !ok {
		m.mu.Unlock()
		return AuthSession{}, fmt.Errorf("authentication session not found or expired")
	}
	if session.view.State != AuthSessionAwaitingCode {
		view := session.view
		m.mu.Unlock()
		return view, fmt.Errorf("authentication session is not awaiting a redirect URL")
	}
	flow, ok := session.authenticator.(auth.AuthCodeFlow)
	if !ok {
		m.mu.Unlock()
		return AuthSession{}, fmt.Errorf("authentication session does not support auth_code")
	}
	ctx, cancel := context.WithDeadline(m.rootCtx, session.view.ExpiresAt)
	m.mu.Unlock()
	defer cancel()
	if err := flow.ExchangeCode(ctx, redirectURL, session.scopes); err != nil {
		m.failSession(id, err)
		view, _ := m.Session(id)
		return view, err
	}
	if credential, ok := session.authenticator.(*auth.AuthCodeCredential); ok {
		if err := credential.PersistAccount(session.authRecordPath); err != nil {
			m.failSession(id, err)
			view, _ := m.Session(id)
			return view, err
		}
	}
	if err := m.finishAuthentication(ctx, id); err != nil {
		m.failSession(id, err)
		view, _ := m.Session(id)
		return view, err
	}
	view, _ := m.Session(id)
	return view, nil
}

// CancelAuthentication cancels an active process-bound session. Completed and
// failed sessions remain readable until expiry. It returns false when id is
// unknown and performs no token revocation.
func (m *Module) CancelAuthentication(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	session, ok := m.sessions[id]
	if !ok {
		return false
	}
	if activeSession(session.view.State) {
		session.cancel()
		session.view.State = AuthSessionCancelled
		session.view.Error = "authentication cancelled"
	}
	return true
}

// runInteractiveAuthentication drives browser or device-code auth and records
// device instructions while the provider call remains active.
func (m *Module) runInteractiveAuthentication(ctx context.Context, id, method string) {
	m.mu.Lock()
	session := m.sessions[id]
	m.mu.Unlock()
	if session == nil {
		return
	}
	prompt := make(chan string, 1)
	details := make(chan auth.DeviceCodeDetails, 1)
	authCtx := ctx
	if method == "device_code" {
		authCtx = context.WithValue(ctx, auth.DeviceCodeMsgKey, prompt)
		authCtx = context.WithValue(authCtx, auth.DeviceCodeDetailsKey, details)
	}
	result := make(chan error, 1)
	go func() {
		_, err := auth.Authenticate(authCtx, session.authenticator, session.authRecordPath, session.scopes)
		result <- err
	}()
	for {
		select {
		case message := <-prompt:
			m.mu.Lock()
			if current := m.sessions[id]; current != nil {
				current.view.Prompt = message
			}
			m.mu.Unlock()
		case instruction := <-details:
			m.mu.Lock()
			if current := m.sessions[id]; current != nil {
				current.view.VerificationURL = instruction.VerificationURL
				current.view.UserCode = instruction.UserCode
				current.view.Prompt = instruction.Message
			}
			m.mu.Unlock()
		case err := <-result:
			if err != nil {
				m.failSession(id, err)
				return
			}
			if err := m.finishAuthentication(ctx, id); err != nil {
				m.failSession(id, err)
			}
			return
		case <-ctx.Done():
			m.failSession(id, ctx.Err())
			return
		}
	}
}

// finishAuthentication creates the scoped Graph client, clears the durable
// re-auth marker, and publishes the authenticated runtime account atomically.
func (m *Module) finishAuthentication(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	session := m.sessions[id]
	if session == nil {
		return fmt.Errorf("authentication session expired")
	}
	client, err := m.clientFactory(session.credential, session.scopes)
	if err != nil {
		return fmt.Errorf("create Graph client: %w", err)
	}
	if err := auth.SetAccountReauthenticationRequired(m.cfg.AccountsPath, session.view.Label, false); err != nil {
		return fmt.Errorf("persist authentication state: %w", err)
	}
	if err := m.registry.Update(session.view.Label, func(entry *auth.AccountEntry) {
		entry.Credential = session.credential
		entry.Authenticator = session.authenticator
		entry.AuthRecordPath = session.authRecordPath
		entry.CacheName = session.cacheName
		entry.Client = client
		entry.Scopes = append([]string(nil), session.scopes...)
		entry.Authenticated = true
		entry.ReauthenticationRequired = false
		entry.TokenTenantContext = auth.TokenTenantContextFromAuthState(entry.AuthMethod, session.authRecordPath)
		entry.Email = ""
	}); err != nil {
		return err
	}
	entry, _ := m.registry.Get(session.view.Label)
	if entry != nil {
		auth.EnsureEmailAndPersistUPN(ctx, entry, m.cfg.AccountsPath)
		if entry.Email != "" {
			_ = m.registry.PublishEmail(entry.Label, entry.AccountID, entry.Email)
		}
	}
	session.view.State = AuthSessionComplete
	session.view.Prompt = "Authentication complete."
	session.view.AuthURL = ""
	session.view.VerificationURL = ""
	session.view.UserCode = ""
	session.view.Error = ""
	session.cancel()
	return nil
}

// failSession records a sanitized provider error on an existing session.
func (m *Module) failSession(id string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if session := m.sessions[id]; session != nil {
		if session.view.State == AuthSessionCancelled {
			return
		}
		session.view.State = AuthSessionFailed
		session.view.Error = err.Error()
		session.cancel()
	}
}

// expireSessionsLocked cancels and removes expired sessions. The caller must
// hold m.mu. It also releases per-label active-session indexes.
func (m *Module) expireSessionsLocked() {
	now := m.now()
	for id, session := range m.sessions {
		if now.Before(session.view.ExpiresAt) {
			continue
		}
		session.cancel()
		delete(m.sessions, id)
		if m.sessionByLabel[session.view.Label] == id {
			delete(m.sessionByLabel, session.view.Label)
		}
	}
}

// activeSession reports whether state represents user/provider work in flight.
func activeSession(state AuthSessionState) bool {
	return state == AuthSessionStarting || state == AuthSessionWaiting || state == AuthSessionAwaitingCode
}
