package webui

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/base64"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/desek/outlook-local-mcp/internal/accountadmin"
	"github.com/desek/outlook-local-mcp/internal/config"
	"github.com/desek/outlook-local-mcp/internal/observability"
	"go.opentelemetry.io/otel/trace"
)

//go:embed templates/*.html static/*
var content embed.FS

// Server is the loopback HTTP adapter and its security state.
type Server struct {
	cfg      config.Config
	admin    *accountadmin.Module
	baseURL  string
	host     string
	csrf     string
	template *template.Template
	http     *http.Server
	metrics  *observability.ToolMetrics
	tracer   trace.Tracer
}

// New creates a secured embedded web server for baseURL. It returns an error
// if templates, static assets, or CSRF entropy cannot be initialized. It does
// not bind a socket or start goroutines.
func New(cfg config.Config, admin *accountadmin.Module, baseURL string) (*Server, error) {
	return NewObserved(cfg, admin, baseURL, nil, nil)
}

// NewObserved creates the secured server and attaches the same metrics and
// tracing identities used by MCP account verbs. Nil observers disable telemetry.
func NewObserved(cfg config.Config, admin *accountadmin.Module, baseURL string, metrics *observability.ToolMetrics, tracer trace.Tracer) (*Server, error) {
	csrfBytes := make([]byte, 32)
	if _, err := rand.Read(csrfBytes); err != nil {
		return nil, fmt.Errorf("generate CSRF token: %w", err)
	}
	tmpl, err := template.New("index.html").Funcs(template.FuncMap{
		"calendarAlias": calendarAliasFromDisplayName,
	}).ParseFS(content, "templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("parse embedded templates: %w", err)
	}
	host := strings.TrimPrefix(baseURL, "http://")
	server := &Server{
		cfg: cfg, admin: admin, baseURL: baseURL, host: host,
		csrf: base64.RawURLEncoding.EncodeToString(csrfBytes), template: tmpl,
		metrics: metrics, tracer: tracer,
	}
	server.http = &http.Server{
		Handler:           server.routes(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    32 << 10,
	}
	return server, nil
}

// Serve runs the HTTP adapter on an already-bound loopback listener. It blocks
// until shutdown or a server failure. http.ErrServerClosed is normalized to nil.
func (s *Server) Serve(listener net.Listener) error {
	err := s.http.Serve(listener)
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

// Shutdown gracefully stops the HTTP adapter within ctx. It does not cancel
// account authentication directly; the shared process root context does that.
func (s *Server) Shutdown(ctx context.Context) error { return s.http.Shutdown(ctx) }

// routes builds the fixed route table and wraps it in security validation.
func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", s.handleIndex)
	mux.HandleFunc("POST /accounts", s.handleCreateAccount)
	mux.HandleFunc("POST /accounts/{label}/permissions", s.handlePermissions)
	mux.HandleFunc("POST /accounts/{label}/reauth", s.handleReauthentication)
	mux.HandleFunc("POST /accounts/{label}/logout", s.handleLogout)
	mux.HandleFunc("POST /accounts/{label}/remove", s.handleRemove)
	mux.HandleFunc("POST /accounts/{label}/shared/refresh", s.handleSharedRefresh)
	mux.HandleFunc("POST /accounts/{label}/calendars", s.handleAddCalendar)
	mux.HandleFunc("POST /accounts/{label}/calendars/{alias}/profile", s.handleCalendarProfile)
	mux.HandleFunc("POST /accounts/{label}/mailboxes", s.handleAddMailbox)
	mux.HandleFunc("POST /accounts/{label}/mailboxes/{alias}/policy", s.handleMailboxPolicy)
	mux.HandleFunc("POST /accounts/{label}/shared/{family}/{alias}/remove", s.handleRemoveShared)
	mux.HandleFunc("GET /auth/{id}", s.handleAuthStatus)
	mux.HandleFunc("POST /auth/{id}/complete", s.handleAuthComplete)
	mux.HandleFunc("POST /auth/{id}/cancel", s.handleAuthCancel)
	staticFS, err := fs.Sub(content, "static")
	if err == nil {
		mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))
	}
	return s.securityHeaders(s.validateRequest(mux))
}

// validateRequest rejects DNS-rebinding hosts and cross-origin mutations.
func (s *Server) validateRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if subtle.ConstantTimeCompare([]byte(r.Host), []byte(s.host)) != 1 {
			http.Error(w, "invalid host", http.StatusBadRequest)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			if subtle.ConstantTimeCompare([]byte(r.Header.Get("Origin")), []byte(s.baseURL)) != 1 {
				http.Error(w, "invalid origin", http.StatusForbidden)
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
			if err := parseMutationForm(r); err != nil {
				http.Error(w, "invalid form", http.StatusBadRequest)
				return
			}
			if r.MultipartForm != nil {
				defer func() { _ = r.MultipartForm.RemoveAll() }()
			}
			if subtle.ConstantTimeCompare([]byte(r.Form.Get("csrf")), []byte(s.csrf)) != 1 {
				http.Error(w, "invalid CSRF token", http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// parseMutationForm parses both standard HTML form encoding and the multipart
// encoding emitted by JavaScript FormData. The request body must already be
// wrapped in an HTTP size limiter. It populates r.Form and may allocate
// multipart bookkeeping that the caller must remove after the request.
func parseMutationForm(r *http.Request) error {
	err := r.ParseMultipartForm(64 << 10)
	if errors.Is(err, http.ErrNotMultipart) {
		return nil
	}
	return err
}

// securityHeaders applies a restrictive browser policy and disables caching of
// account/authentication state. It does not alter response bodies.
func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'; object-src 'none'")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		next.ServeHTTP(w, r)
	})
}
