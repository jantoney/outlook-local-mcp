package accountadmin

import (
	"time"

	"github.com/desek/outlook-local-mcp/internal/auth"
	"github.com/desek/outlook-local-mcp/internal/resource"
)

// Account is a detached administration view safe for rendering or serializing.
// It intentionally excludes credentials, tokens, authenticators, and clients.
type Account struct {
	AccountID                auth.AccountID
	Label                    string
	UPN                      string
	ClientID                 string
	TenantID                 string
	AuthMethod               string
	CalendarPolicy           auth.CalendarPolicy
	MailPolicy               auth.MailActionPolicy
	CalendarAliases          []resource.CalendarAlias
	MailAliases              []resource.MailAlias
	Authenticated            bool
	ReauthenticationRequired bool
	ActiveScopes             []string
	RequiredScopes           []string
	CalendarCandidates       []CalendarCandidate
}

// PermissionUpdate contains the complete desired own-resource policy for one
// account. Callers must send both policies so the module can calculate one
// deterministic before-and-after scope union.
type PermissionUpdate struct {
	Calendar auth.CalendarPolicy
	Mail     auth.MailActionPolicy
}

// CreateRequest contains immutable identity and initial permission choices for
// a new disconnected account. Empty identity fields are filled from configured
// add-account defaults by Module.CreateAccount.
type CreateRequest struct {
	Label          string
	ClientID       string
	TenantID       string
	AuthMethod     string
	CalendarPolicy auth.CalendarPolicy
	MailPolicy     auth.MailActionPolicy
}

// AuthSessionState identifies the process-bound state of an interactive
// authentication attempt.
type AuthSessionState string

const (
	// AuthSessionStarting indicates credential setup or provider initialization.
	AuthSessionStarting AuthSessionState = "starting"
	// AuthSessionWaiting indicates the user must complete browser/device login.
	AuthSessionWaiting AuthSessionState = "waiting"
	// AuthSessionAwaitingCode indicates auth_code requires a redirect URL.
	AuthSessionAwaitingCode AuthSessionState = "awaiting_code"
	// AuthSessionComplete indicates the account has a current Graph client.
	AuthSessionComplete AuthSessionState = "complete"
	// AuthSessionFailed indicates authentication or client setup failed.
	AuthSessionFailed AuthSessionState = "failed"
	// AuthSessionCancelled indicates an explicit cancellation or process stop.
	AuthSessionCancelled AuthSessionState = "cancelled"
)

// AuthSession is a secret-free snapshot returned to adapters. AuthURL and
// Prompt contain user-facing provider instructions but never tokens or PKCE
// verifier material.
type AuthSession struct {
	ID              string           `json:"id"`
	Label           string           `json:"label"`
	State           AuthSessionState `json:"state"`
	AuthURL         string           `json:"auth_url,omitempty"`
	// VerificationURL is the device-login page supplied by Microsoft.
	VerificationURL string           `json:"verification_url,omitempty"`
	// UserCode is the short-lived device code entered at VerificationURL.
	UserCode        string           `json:"user_code,omitempty"`
	Prompt          string           `json:"prompt,omitempty"`
	Error           string           `json:"error,omitempty"`
	CreatedAt       time.Time        `json:"created_at"`
	ExpiresAt       time.Time        `json:"expires_at"`
}
