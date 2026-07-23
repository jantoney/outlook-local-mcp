package webui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/desek/outlook-local-mcp/internal/accountadmin"
	"github.com/desek/outlook-local-mcp/internal/audit"
	"github.com/desek/outlook-local-mcp/internal/auth"
	"github.com/desek/outlook-local-mcp/internal/observability"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// pageData is the complete server-rendered account administration view.
type pageData struct {
	Accounts          []accountadmin.Account
	CSRF              string
	ReadOnly          bool
	BaseURL           string
	DefaultClientID   string
	DefaultTenantID   string
	DefaultAuthMethod string
	Notice            string
	Problem           string
	Session           *accountadmin.AuthSession
}

// handleIndex renders the single account administration page. It performs no
// Graph calls and never includes credentials or token claims in template data.
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	data := pageData{
		Accounts: s.admin.Accounts(), CSRF: s.csrf, ReadOnly: s.cfg.ReadOnly,
		BaseURL: s.baseURL, DefaultClientID: s.cfg.ClientID,
		DefaultTenantID: s.cfg.TenantID, DefaultAuthMethod: s.cfg.AuthMethod,
		Notice: r.URL.Query().Get("notice"), Problem: r.URL.Query().Get("error"),
	}
	if id := r.URL.Query().Get("session"); id != "" {
		if session, ok := s.admin.Session(id); ok {
			data.Session = &session
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.template.ExecuteTemplate(w, "index.html", data); err != nil {
		http.Error(w, "render account page", http.StatusInternalServerError)
	}
}

// handleCreateAccount validates a form and creates a disconnected account with
// explicit initial permissions. Authentication starts only when the user
// presses Re-authenticate.
func (s *Server) handleCreateAccount(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	label := strings.TrimSpace(r.Form.Get("label"))
	calendar, err := auth.ParseCalendarPolicy(r.Form.Get("calendar_policy"))
	if err == nil {
		_, err = s.admin.CreateAccount(accountadmin.CreateRequest{
			Label: label, ClientID: strings.TrimSpace(r.Form.Get("client_id")),
			TenantID:   strings.TrimSpace(r.Form.Get("tenant_id")),
			AuthMethod: r.Form.Get("auth_method"), CalendarPolicy: calendar,
			MailPolicy: mailPolicyFromForm(r),
		})
	}
	s.audit("account.add", "write", label, started, err)
	if err != nil {
		s.redirect(w, r, "", err)
		return
	}
	s.redirect(w, r, fmt.Sprintf("Account %q configured. Review permissions, then re-authenticate.", label), nil)
}

// handlePermissions replaces an account's own-resource policy immediately.
// The shared module decides whether the scope union requires re-authentication.
func (s *Server) handlePermissions(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	label := r.PathValue("label")
	calendar, err := auth.ParseCalendarPolicy(r.Form.Get("calendar_policy"))
	if err == nil {
		_, err = s.admin.SetPermissions(label, accountadmin.PermissionUpdate{
			Calendar: calendar, Mail: mailPolicyFromForm(r),
		})
	}
	s.audit("account.set_permissions", "write", label, started, err)
	if err != nil {
		s.redirect(w, r, "", err)
		return
	}
	s.redirect(w, r, fmt.Sprintf("Permissions for %q saved.", label), nil)
}

// handleReauthentication starts an in-memory account authentication session and
// redirects to the same page with session progress visible.
func (s *Server) handleReauthentication(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	label := r.PathValue("label")
	var session accountadmin.AuthSession
	var err error
	if id := strings.TrimSpace(r.Form.Get("restart_session")); id != "" {
		session, err = s.admin.RestartAuthentication(label, id)
	} else {
		session, err = s.admin.StartAuthentication(label)
	}
	s.audit("account.login", "write", label, started, err)
	if err != nil {
		s.redirect(w, r, "", err)
		return
	}
	http.Redirect(w, r, "/?session="+url.QueryEscape(session.ID), http.StatusSeeOther)
}

// handleLogout disconnects an account while preserving its configuration.
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	label := r.PathValue("label")
	err := s.admin.Logout(label)
	s.audit("account.logout", "write", label, started, err)
	if err != nil {
		s.redirect(w, r, "", err)
		return
	}
	s.redirect(w, r, fmt.Sprintf("Account %q disconnected.", label), nil)
}

// handleRemove requires the typed label to match the route label before the
// destructive account removal is delegated to the shared module.
func (s *Server) handleRemove(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	label := r.PathValue("label")
	var err error
	if r.Form.Get("confirm_label") != label {
		err = fmt.Errorf("type %q exactly to remove this account", label)
	} else {
		err = s.admin.Remove(label)
	}
	s.audit("account.remove", "delete", label, started, err)
	if err != nil {
		s.redirect(w, r, "", err)
		return
	}
	s.redirect(w, r, fmt.Sprintf("Account %q removed and local authentication cleared.", label), nil)
}

// handleAuthStatus returns a secret-free JSON session snapshot for page polling.
func (s *Server) handleAuthStatus(w http.ResponseWriter, r *http.Request) {
	session, ok := s.admin.Session(r.PathValue("id"))
	if !ok {
		http.Error(w, "authentication session expired", http.StatusGone)
		return
	}
	writeJSON(w, session)
}

// handleAuthComplete accepts an auth_code redirect URL and completes the
// process-bound session. The redirect value is never logged or echoed.
func (s *Server) handleAuthComplete(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	session, err := s.admin.CompleteAuthentication(r.PathValue("id"), r.Form.Get("redirect_url"))
	s.audit("account.complete_auth", "write", session.Label, started, err)
	if err != nil {
		s.redirectToSession(w, r, r.PathValue("id"), err)
		return
	}
	http.Redirect(w, r, "/?session="+url.QueryEscape(session.ID), http.StatusSeeOther)
}

// handleAuthCancel cancels an active authentication session without changing
// account configuration.
func (s *Server) handleAuthCancel(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !s.admin.CancelAuthentication(id) {
		http.Error(w, "authentication session not found", http.StatusNotFound)
		return
	}
	s.redirect(w, r, "Authentication cancelled.", nil)
}

// mailPolicyFromForm maps exact checkbox names to the independent policy.
func mailPolicyFromForm(r *http.Request) auth.MailActionPolicy {
	on := func(name string) bool { return r.Form.Get(name) == "on" }
	return auth.MailActionPolicy{
		Read: on("mail_read"), Draft: on("mail_draft"), Move: on("mail_move"),
		Archive: on("mail_archive"), Trash: on("mail_trash"), Restore: on("mail_restore"),
		PermanentDelete: on("mail_permanent_delete"), Send: on("mail_send"),
	}
}

// redirect implements POST/Redirect/GET with one escaped notice or error.
func (s *Server) redirect(w http.ResponseWriter, r *http.Request, notice string, err error) {
	values := url.Values{}
	if notice != "" {
		values.Set("notice", notice)
	}
	if err != nil {
		values.Set("error", err.Error())
	}
	target := "/"
	if query := values.Encode(); query != "" {
		target += "?" + query
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}

// redirectToSession preserves a session panel while reporting an error.
func (s *Server) redirectToSession(w http.ResponseWriter, r *http.Request, id string, err error) {
	values := url.Values{"session": []string{id}, "error": []string{err.Error()}}
	http.Redirect(w, r, "/?"+values.Encode(), http.StatusSeeOther)
}

// writeJSON serializes value as a no-store JSON response.
func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		http.Error(w, "serialize response", http.StatusInternalServerError)
	}
}

// audit emits the same operation identity used by MCP with transport=web. It
// includes only the local account label and never includes auth redirects.
func (s *Server) audit(operation, operationType, label string, started time.Time, err error) {
	outcome := "success"
	errorMessage := ""
	if err != nil {
		outcome = "error"
		errorMessage = err.Error()
	}
	audit.EmitAuditLog(audit.AuditEntry{
		Audit: true, Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		ToolName: operation, OperationType: operationType,
		Parameters: map[string]string{"transport": "web"}, Account: label,
		Outcome: outcome, DurationMs: time.Since(started).Milliseconds(), ErrorMessage: errorMessage,
	})
	duration := time.Since(started)
	observability.RecordOperation(context.Background(), s.metrics, operation, outcome, duration)
	if s.tracer != nil {
		_, span := s.tracer.Start(context.Background(), operation, trace.WithTimestamp(started), trace.WithAttributes(
			attribute.String("transport", "web"), attribute.String("account", label), attribute.String("status", outcome),
		))
		span.End()
	}
}
