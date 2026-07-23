package accountadmin

import (
	"errors"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/desek/outlook-local-mcp/internal/auth"
)

// TestRestartAuthenticationReplacesWaitingSession verifies that an explicit
// retry cancels a provider transaction instead of resuming its stale code.
func TestRestartAuthenticationReplacesWaitingSession(t *testing.T) {
	module, _ := newPermissionTestModule(t, auth.CalendarPolicyOff, auth.MailActionPolicy{})
	cancelled := false
	module.sessions["stale"] = &authSession{
		view: AuthSession{
			ID: "stale", Label: "work", State: AuthSessionWaiting,
			ExpiresAt: module.now().Add(time.Minute),
		},
		cancel: func() { cancelled = true },
	}
	module.sessionByLabel["work"] = "stale"
	wantErr := errors.New("fresh credential requested")
	module.credentialSetup = func(
		_, _, _, _, _, _, _ string,
	) (azcore.TokenCredential, auth.Authenticator, string, string, error) {
		return nil, nil, "", "", wantErr
	}

	_, err := module.RestartAuthentication("work", "stale")
	if !errors.Is(err, wantErr) {
		t.Fatalf("RestartAuthentication() error = %v, want %v", err, wantErr)
	}
	if !cancelled {
		t.Fatal("RestartAuthentication() did not cancel the waiting provider transaction")
	}
	if got := module.sessions["stale"].view.State; got != AuthSessionCancelled {
		t.Fatalf("stale session state = %q, want %q", got, AuthSessionCancelled)
	}
}

// TestRestartAuthenticationRecoversExpiredSession verifies that a retry from
// a stale browser page can start a fresh provider transaction after the old
// process-bound session has already been removed.
func TestRestartAuthenticationRecoversExpiredSession(t *testing.T) {
	module, _ := newPermissionTestModule(t, auth.CalendarPolicyOff, auth.MailActionPolicy{})
	wantErr := errors.New("fresh credential requested")
	module.credentialSetup = func(
		_, _, _, _, _, _, _ string,
	) (azcore.TokenCredential, auth.Authenticator, string, string, error) {
		return nil, nil, "", "", wantErr
	}

	_, err := module.RestartAuthentication("work", "expired-session")
	if !errors.Is(err, wantErr) {
		t.Fatalf("RestartAuthentication() error = %v, want %v", err, wantErr)
	}
}
