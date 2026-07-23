package webui

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/desek/outlook-local-mcp/internal/resource"
)

// handleSharedRefresh performs observational discovery and direct validation.
func (s *Server) handleSharedRefresh(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	label := r.PathValue("label")
	result, err := s.admin.RefreshSharedResources(r.Context(), label)
	s.audit("account.refresh_shared_resources", "read", label, started, err)
	if err != nil {
		s.redirect(w, r, "", err)
		return
	}
	notice := fmt.Sprintf("Shared resources refreshed; %d calendar candidates discovered.", len(result.CalendarCandidates))
	if result.DiscoveryError != "" {
		notice = "Configured resources were revalidated, but calendar discovery was unavailable. Manual entry remains available."
	}
	s.redirect(w, r, notice, nil)
}

// handleAddCalendar configures a manual owner-primary or mounted calendar.
func (s *Server) handleAddCalendar(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	label := r.PathValue("label")
	alias := calendarAliasFromDisplayName(r.Form.Get("alias"))
	kind := resource.CalendarKind(r.Form.Get("kind"))
	profile := resource.CalendarProfile(r.Form.Get("profile"))
	_, err := s.admin.AddCalendar(label, alias, strings.TrimSpace(r.Form.Get("owner")), kind, strings.TrimSpace(r.Form.Get("mounted_id")), profile)
	s.audit("account.add_calendar_alias", "write", label, started, err)
	if err != nil {
		s.redirect(w, r, "", err)
		return
	}
	s.redirect(w, r, fmt.Sprintf("Shared calendar %q configured. Authenticate if required, then refresh to validate it.", alias), nil)
}

// handleCalendarProfile changes one calendar alias's local policy.
func (s *Server) handleCalendarProfile(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	label := r.PathValue("label")
	_, err := s.admin.SetCalendarProfile(label, r.PathValue("alias"), resource.CalendarProfile(r.Form.Get("profile")))
	s.audit("account.set_calendar_alias_profile", "write", label, started, err)
	if err != nil {
		s.redirect(w, r, "", err)
		return
	}
	s.redirect(w, r, "Shared calendar permission saved.", nil)
}

// handleAddMailbox configures an all-actions-off manual shared mailbox.
func (s *Server) handleAddMailbox(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	label := r.PathValue("label")
	_, err := s.admin.AddMailbox(label, strings.TrimSpace(r.Form.Get("alias")), strings.TrimSpace(r.Form.Get("owner")))
	s.audit("account.add_mail_alias", "write", label, started, err)
	if err != nil {
		s.redirect(w, r, "", err)
		return
	}
	s.redirect(w, r, "Shared mailbox configured with all actions off. Enable permissions, authenticate, then refresh to validate it.", nil)
}

// handleMailboxPolicy replaces one shared mailbox's exact local action matrix.
func (s *Server) handleMailboxPolicy(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	label := r.PathValue("label")
	_, err := s.admin.SetMailboxPolicy(label, r.PathValue("alias"), mailPolicyFromForm(r))
	s.audit("account.set_mail_alias_policy", "write", label, started, err)
	if err != nil {
		s.redirect(w, r, "", err)
		return
	}
	s.redirect(w, r, "Shared mailbox permissions saved.", nil)
}

// handleRemoveShared requires an exact alias confirmation before removal.
func (s *Server) handleRemoveShared(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	label, alias := r.PathValue("label"), r.PathValue("alias")
	var err error
	if r.Form.Get("confirm_alias") != alias {
		err = fmt.Errorf("confirm alias %q exactly", alias)
	} else {
		_, err = s.admin.RemoveSharedResource(label, r.PathValue("family"), alias)
	}
	s.audit("account.remove_shared_resource", "delete", label, started, err)
	if err != nil {
		s.redirect(w, r, "", err)
		return
	}
	s.redirect(w, r, fmt.Sprintf("Shared resource %q removed.", alias), nil)
}
