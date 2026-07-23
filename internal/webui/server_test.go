package webui

import (
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/desek/outlook-local-mcp/internal/accountadmin"
	"github.com/desek/outlook-local-mcp/internal/auth"
	"github.com/desek/outlook-local-mcp/internal/config"
)

// newTestServer creates a loopback web adapter without binding a real port.
func newTestServer(t *testing.T) *Server {
	t.Helper()
	dir := t.TempDir()
	cfg := config.Config{ClientID: "client", TenantID: "tenant", AuthMethod: "browser", AccountsPath: filepath.Join(dir, "accounts.json")}
	server, err := New(cfg, accountadmin.New(context.Background(), cfg, auth.NewAccountRegistry()), "http://127.0.0.1:8155")
	if err != nil {
		t.Fatal(err)
	}
	return server
}

// TestCreateAccountPost verifies the secured form adapter delegates to the
// shared administration module and redirects with a persisted account.
func TestCreateAccountPost(t *testing.T) {
	server := newTestServer(t)
	form := url.Values{
		"csrf": {server.csrf}, "label": {"work"}, "client_id": {"client"},
		"tenant_id": {"tenant"}, "auth_method": {"browser"}, "calendar_policy": {"off"},
	}
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8155/accounts", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Origin", "http://127.0.0.1:8155")
	response := httptest.NewRecorder()
	server.http.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("POST /accounts = %d, body %q", response.Code, response.Body.String())
	}
	if accounts := server.admin.Accounts(); len(accounts) != 1 || accounts[0].Label != "work" {
		t.Fatalf("created accounts = %+v", accounts)
	}
}

// TestIndexRendersEmptyAccountPage verifies the embedded template and assets
// initialize without a frontend build chain.
func TestIndexRendersEmptyAccountPage(t *testing.T) {
	server := newTestServer(t)
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8155/", nil)
	response := httptest.NewRecorder()
	server.http.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "No accounts configured") {
		t.Fatalf("GET / = %d, body %q", response.Code, response.Body.String())
	}
}

// TestIndexExplainsMountedCalendarIDLookup verifies the manual shared-calendar
// form explains both in-product discovery and the Graph fallback ID source.
func TestIndexExplainsMountedCalendarIDLookup(t *testing.T) {
	server := newTestServer(t)
	if _, err := server.admin.CreateAccount(accountadmin.CreateRequest{
		Label: "personal", ClientID: "client", TenantID: "tenant",
		AuthMethod: "browser", CalendarPolicy: auth.CalendarPolicyRead,
	}); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8155/", nil)
	response := httptest.NewRecorder()
	server.http.Handler.ServeHTTP(response, request)
	body := response.Body.String()
	for _, text := range []string{"Use Refresh first", "GET /me/calendars", "calendar object’s <code>id</code>"} {
		if !strings.Contains(body, text) {
			t.Fatalf("account page does not explain mounted calendar ID lookup; missing %q", text)
		}
	}
}

// TestHostValidationRejectsDNSRebinding verifies an attacker-controlled Host
// header cannot reach even read-only UI routes.
func TestHostValidationRejectsDNSRebinding(t *testing.T) {
	server := newTestServer(t)
	request := httptest.NewRequest(http.MethodGet, "http://evil.example/", nil)
	request.Host = "evil.example"
	response := httptest.NewRecorder()
	server.http.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid Host status = %d, want 400", response.Code)
	}
}

// TestMutationRequiresOriginAndCSRF verifies cross-origin form posts fail before
// the account administration module is invoked.
func TestMutationRequiresOriginAndCSRF(t *testing.T) {
	server := newTestServer(t)
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8155/accounts", strings.NewReader("csrf="+server.csrf))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Origin", "http://evil.example")
	response := httptest.NewRecorder()
	server.http.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("cross-origin mutation status = %d, want 403", response.Code)
	}
}

// TestSecurityHeadersPreserveSameOriginFormOrigin verifies the referrer policy
// does not force Chromium to serialize native POST form origins as null.
func TestSecurityHeadersPreserveSameOriginFormOrigin(t *testing.T) {
	server := newTestServer(t)
	handler := server.securityHeaders(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8155/", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if policy := response.Header().Get("Referrer-Policy"); policy != "same-origin" {
		t.Fatalf("Referrer-Policy = %q, want same-origin", policy)
	}
}

// TestMultipartAutosaveAcceptsCSRF verifies that the security adapter parses
// the multipart encoding emitted by FormData before validating its CSRF field.
func TestMultipartAutosaveAcceptsCSRF(t *testing.T) {
	server := newTestServer(t)
	if _, err := server.admin.CreateAccount(accountadmin.CreateRequest{
		Label: "personal", ClientID: "client", TenantID: "tenant",
		AuthMethod: "browser", CalendarPolicy: auth.CalendarPolicyOff,
	}); err != nil {
		t.Fatal(err)
	}

	var body strings.Builder
	writer := multipart.NewWriter(&body)
	for name, value := range map[string]string{
		"csrf": server.csrf, "calendar_policy": string(auth.CalendarPolicyManage),
	} {
		if err := writer.WriteField(name, value); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8155/accounts/personal/permissions", strings.NewReader(body.String()))
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("Origin", "http://127.0.0.1:8155")
	response := httptest.NewRecorder()
	server.http.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("multipart permissions status = %d, body %q", response.Code, response.Body.String())
	}
	accounts := server.admin.Accounts()
	if len(accounts) != 1 || accounts[0].CalendarPolicy != auth.CalendarPolicyManage {
		t.Fatalf("multipart permissions did not persist manage policy: %+v", accounts)
	}
}

// TestCalendarAliasFromDisplayName verifies human calendar names become valid,
// predictable local aliases without asking users to understand alias syntax.
func TestCalendarAliasFromDisplayName(t *testing.T) {
	tests := map[string]string{
		"Australian Public Holidays":  "australian-public-holidays",
		"  Team / Release Calendar  ": "team-release-calendar",
		"日本の祝日":                       "calendar",
	}
	for input, want := range tests {
		if got := calendarAliasFromDisplayName(input); got != want {
			t.Errorf("calendarAliasFromDisplayName(%q) = %q, want %q", input, got, want)
		}
	}
}

// TestInteractiveCalendarAndAuthenticationMarkup verifies the page exposes
// direct candidate configuration plus copyable structured auth instructions.
func TestInteractiveCalendarAndAuthenticationMarkup(t *testing.T) {
	server := newTestServer(t)
	data := pageData{
		CSRF: "csrf", BaseURL: "http://127.0.0.1:8155",
		Session: &accountadmin.AuthSession{
			ID: "session", Label: "personal", State: accountadmin.AuthSessionWaiting,
			VerificationURL: "https://microsoft.com/devicelogin", UserCode: "ABCD-EFGH",
		},
		Accounts: []accountadmin.Account{{
			Label: "personal", Authenticated: true,
			CalendarCandidates: []accountadmin.CalendarCandidate{{
				ID: "holiday-id", Name: "Australian Public Holidays", Owner: "owner@example.com", CanEdit: true,
			}},
		}},
	}
	var body strings.Builder
	if err := server.template.ExecuteTemplate(&body, "index.html", data); err != nil {
		t.Fatal(err)
	}
	markup := body.String()
	for _, want := range []string{
		`href="https://microsoft.com/devicelogin"`, `data-copy-value="https://microsoft.com/devicelogin"`,
		`data-copy-value="ABCD-EFGH"`, `value="australian-public-holidays"`,
		`value="holiday-id"`, `data-refresh-on-open`, `data-detail-key="account:personal"`,
	} {
		if !strings.Contains(markup, want) {
			t.Errorf("interactive page markup missing %q", want)
		}
	}
}

// TestFrontendAssetsPreserveDisclosureState verifies the no-build frontend
// contains the contracts that prevent hidden layout overlap and collapsing
// account state during background updates.
func TestFrontendAssetsPreserveDisclosureState(t *testing.T) {
	tests := map[string][]string{
		"static/app.css": {`details:not([open]) > :not(summary)`, `[hidden]`},
		"static/app.js": {
			`sessionStorage`, `data-refresh-form`, `card.replaceWith(nextCard)`,
			`response.status === 410`, `Authentication session expired. Get a new sign-in code.`,
		},
	}
	for name, expected := range tests {
		contents, err := content.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range expected {
			if !strings.Contains(string(contents), want) {
				t.Errorf("%s missing frontend contract %q", name, want)
			}
		}
	}
}

// TestMissingAuthenticationSessionReturnsGone verifies polling can distinguish
// an expired process-bound session from a transient server failure.
func TestMissingAuthenticationSessionReturnsGone(t *testing.T) {
	server := newTestServer(t)
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8155/auth/expired-session", nil)
	response := httptest.NewRecorder()
	server.http.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusGone {
		t.Fatalf("GET /auth/expired-session = %d, want %d", response.Code, http.StatusGone)
	}
}
