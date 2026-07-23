package accountadmin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/desek/outlook-local-mcp/internal/auth"
	"github.com/desek/outlook-local-mcp/internal/config"
	"github.com/desek/outlook-local-mcp/internal/resource"
	kiotaauth "github.com/microsoft/kiota-abstractions-go/authentication"
	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"
)

// validationTransport redirects generated Graph URLs to a local test server.
type validationTransport struct{ baseURL string }

// RoundTrip preserves the generated path while replacing only origin fields.
func (transport validationTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	request.URL.Scheme = "http"
	request.URL.Host = transport.baseURL[len("http://"):]
	return http.DefaultTransport.RoundTrip(request)
}

// TestRefreshSharedResourcesUsesDirectMetadataRoutes verifies discovery and
// direct validation read only the exact calendar and mailbox metadata routes.
func TestRefreshSharedResourcesUsesDirectMetadataRoutes(t *testing.T) {
	t.Parallel()
	paths := make(chan string, 3)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		paths <- request.URL.EscapedPath()
		writer.Header().Set("Content-Type", "application/json")
		if request.URL.Path == "/v1.0/users/me-token-to-replace/calendars" {
			_, _ = writer.Write([]byte(`{"value":[]}`))
			return
		}
		_, _ = writer.Write([]byte(`{"id":"validated"}`))
	}))
	defer server.Close()
	adapter, err := msgraphsdk.NewGraphRequestAdapterWithParseNodeFactoryAndSerializationWriterFactoryAndHttpClient(
		&kiotaauth.AnonymousAuthenticationProvider{}, nil, nil,
		&http.Client{Transport: validationTransport{baseURL: server.URL}},
	)
	if err != nil {
		t.Fatal(err)
	}
	client := msgraphsdk.NewGraphServiceClient(adapter)

	calendar, err := resource.NewMountedCalendar("11111111-1111-4111-8111-111111111111", "team", "owner@example.com", "mounted-1", resource.CalendarProfileRead)
	if err != nil {
		t.Fatal(err)
	}
	mailbox, err := resource.NewMailAlias("22222222-2222-4222-8222-222222222222", "finance", "finance@example.com")
	if err != nil {
		t.Fatal(err)
	}
	mailbox.Policy = auth.MailActionPolicy{Read: true}
	calendarPolicy := auth.CalendarPolicyOff
	mailPolicy := auth.MailActionPolicy{}
	dir := t.TempDir()
	cfg := config.Config{AccountsPath: filepath.Join(dir, "accounts.json"), RequestTimeout: time.Second}
	if err := auth.SaveAccounts(cfg.AccountsPath, []auth.AccountConfig{{
		AccountID: "33333333-3333-4333-8333-333333333333", Label: "work", ClientID: "client", TenantID: "tenant", AuthMethod: "browser",
		CalendarPolicy: &calendarPolicy, MailPolicy: &mailPolicy, CalendarAliases: &[]resource.CalendarAlias{calendar}, MailAliases: &[]resource.MailAlias{mailbox},
	}}); err != nil {
		t.Fatal(err)
	}
	registry := auth.NewAccountRegistry()
	if err := registry.Add(&auth.AccountEntry{
		AccountID: "33333333-3333-4333-8333-333333333333", Label: "work", Authenticated: true, Client: client,
		CalendarPolicy: calendarPolicy, CalendarAliases: []resource.CalendarAlias{calendar}, MailAliases: []resource.MailAlias{mailbox},
	}); err != nil {
		t.Fatal(err)
	}
	result, err := New(context.Background(), cfg, registry).RefreshSharedResources(context.Background(), "work")
	if err != nil {
		t.Fatal(err)
	}
	if result.CalendarAliases[0].Validation.Status != resource.ValidationAvailable || result.MailAliases[0].Validation.Status != resource.ValidationAvailable {
		t.Fatalf("validation result = %+v", result)
	}
	got := []string{<-paths, <-paths, <-paths}
	want := []string{"/v1.0/users/me-token-to-replace/calendars", "/v1.0/users/me-token-to-replace/calendars/mounted-1", "/v1.0/users/finance%40example.com/mailFolders/inbox"}
	for _, path := range want {
		if !slices.Contains(got, path) {
			t.Fatalf("Graph paths = %v, missing %s", got, path)
		}
	}
}

// TestValidateOwnPermissionsUsesMetadataOnlyRoutes verifies startup checks
// select only the signed-in user's id and never request messages, events, or
// email-address properties.
func TestValidateOwnPermissionsUsesMetadataOnlyRoutes(t *testing.T) {
	t.Parallel()
	requests := make(chan string, 3)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests <- request.URL.RequestURI()
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/v1.0/users/me-token-to-replace":
			_, _ = writer.Write([]byte(`{"id":"user-id"}`))
		case "/v1.0/users/me-token-to-replace/calendars":
			_, _ = writer.Write([]byte(`{"value":[]}`))
		default:
			_, _ = writer.Write([]byte(`{"id":"inbox"}`))
		}
	}))
	defer server.Close()
	adapter, err := msgraphsdk.NewGraphRequestAdapterWithParseNodeFactoryAndSerializationWriterFactoryAndHttpClient(
		&kiotaauth.AnonymousAuthenticationProvider{}, nil, nil,
		&http.Client{Transport: validationTransport{baseURL: server.URL}},
	)
	if err != nil {
		t.Fatal(err)
	}
	registry := auth.NewAccountRegistry()
	if err := registry.Add(&auth.AccountEntry{
		AccountID: "33333333-3333-4333-8333-333333333333", Label: "work", Authenticated: true,
		Client: msgraphsdk.NewGraphServiceClient(adapter), CalendarPolicy: auth.CalendarPolicyRead,
		MailPolicy: auth.MailActionPolicy{Read: true},
	}); err != nil {
		t.Fatal(err)
	}
	module := New(context.Background(), config.Config{RequestTimeout: time.Second}, registry)
	if err := module.validateOwnPermissions(context.Background(), "work"); err != nil {
		t.Fatal(err)
	}
	got := []string{<-requests, <-requests, <-requests}
	if !slices.Contains(got, "/v1.0/users/me-token-to-replace?%24select=id") {
		t.Fatalf("Graph requests = %v, missing id-only User.Read probe", got)
	}
	if !slices.Contains(got, "/v1.0/users/me-token-to-replace/calendars?%24select=id&%24top=1") {
		t.Fatalf("Graph requests = %v, missing metadata-only calendar probe", got)
	}
	if !slices.Contains(got, "/v1.0/users/me-token-to-replace/mailFolders/inbox") {
		t.Fatalf("Graph requests = %v, missing inbox metadata probe", got)
	}
}
