package accountadmin

import "fmt"

// RestartAuthentication cancels the identified authentication attempt for
// label and starts a new provider transaction. The session ID prevents a stale
// browser form from cancelling a newer attempt for the same account. It returns
// the new secret-free session snapshot. A missing session is treated as already
// expired and starts or resumes the label's current attempt. The method returns
// an error when the account is unknown, the session belongs to another account,
// or new credential setup fails. Cancellation affects only process-bound
// authentication state; it does not revoke tokens or alter configuration.
func (m *Module) RestartAuthentication(label, id string) (AuthSession, error) {
	m.mu.Lock()
	session, ok := m.sessions[id]
	if !ok {
		m.mu.Unlock()
		return m.StartAuthentication(label)
	}
	if session.view.Label != label {
		m.mu.Unlock()
		return AuthSession{}, fmt.Errorf("authentication session does not belong to account %q", label)
	}
	if activeSession(session.view.State) {
		session.cancel()
		session.view.State = AuthSessionCancelled
		session.view.Error = "authentication restarted"
	}
	m.mu.Unlock()

	return m.StartAuthentication(label)
}
