package graph

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/desek/outlook-local-mcp/internal/resource"
	kiotaauth "github.com/microsoft/kiota-abstractions-go/authentication"
	kiotahttp "github.com/microsoft/kiota-http-go"
	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"
	msgraphcore "github.com/microsoftgraph/msgraph-sdk-go-core"
)

// TestSelectUserRootRealSDKRoutes verifies the generated SDK produces exact
// escaped Me and Users.ByUserId paths for encoded owner and message IDs.
func TestSelectUserRootRealSDKRoutes(t *testing.T) {
	paths := make(chan string, 3)
	client, server := newTargetTestGraphClient(t, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		paths <- request.URL.EscapedPath()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"message"}`))
	}))
	defer server.Close()
	accountID := resource.AccountID("11111111-1111-4111-8111-111111111111")

	own, err := resource.NewOwnTarget(accountID, resource.TargetFamilyMail, resource.MailActionPolicy{Read: true})
	if err != nil {
		t.Fatalf("NewOwnTarget() error = %v", err)
	}
	root, err := SelectUserRoot(client, own)
	if err != nil {
		t.Fatalf("SelectUserRoot(own) error = %v", err)
	}
	if _, err := root.Messages().ByMessageId("id+with space?").Get(context.Background(), nil); err != nil {
		t.Fatalf("own message Get() error = %v", err)
	}

	resourceID := resource.ResourceID("22222222-2222-4222-8222-222222222222")
	alias, err := resource.NewMailAlias(resourceID, "finance", "finance+ops@example.com")
	if err != nil {
		t.Fatalf("NewMailAlias() error = %v", err)
	}
	alias.Policy = resource.MailActionPolicy{Read: true}
	owner, err := resource.TargetFromMailAlias(accountID, alias)
	if err != nil {
		t.Fatalf("TargetFromMailAlias() error = %v", err)
	}
	root, err = SelectUserRoot(client, owner)
	if err != nil {
		t.Fatalf("SelectUserRoot(owner) error = %v", err)
	}
	if _, err := root.Messages().ByMessageId("id+with space?").Get(context.Background(), nil); err != nil {
		t.Fatalf("owner message Get() error = %v", err)
	}

	mountedAlias, err := resource.NewMountedCalendar(resourceID, "team", "owner@example.com", "calendar+mounted", resource.CalendarProfileRead)
	if err != nil {
		t.Fatalf("NewMountedCalendar() error = %v", err)
	}
	mounted, err := resource.TargetFromCalendarAlias(accountID, mountedAlias)
	if err != nil {
		t.Fatalf("TargetFromCalendarAlias() error = %v", err)
	}
	root, err = SelectUserRoot(client, mounted)
	if err != nil {
		t.Fatalf("SelectUserRoot(mounted) error = %v", err)
	}
	if _, err := root.Calendars().ByCalendarId("calendar+mounted").Get(context.Background(), nil); err != nil {
		t.Fatalf("mounted calendar Get() error = %v", err)
	}

	gotOwn := <-paths
	gotOwner := <-paths
	gotMounted := <-paths
	if gotOwn != "/v1.0/me/messages/id+with%20space%3F" {
		t.Fatalf("own escaped path = %q", gotOwn)
	}
	if gotOwner != "/v1.0/users/finance%2Bops%40example.com/messages/id%2Bwith%20space%3F" {
		t.Fatalf("owner escaped path = %q", gotOwner)
	}
	if gotMounted != "/v1.0/me/calendars/calendar+mounted" {
		t.Fatalf("mounted escaped path = %q", gotMounted)
	}
}

// TestSelectUserRootRejectsInvalidTargetsWithoutTraffic verifies nil clients,
// zero targets, kind/view mismatch, missing owner, and missing mounted IDs fail
// before transport is reachable.
func TestSelectUserRootRejectsInvalidTargetsWithoutTraffic(t *testing.T) {
	var calls atomic.Int32
	client, server := newTargetTestGraphClient(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		calls.Add(1)
	}))
	defer server.Close()
	accountID := resource.AccountID("11111111-1111-4111-8111-111111111111")
	resourceID := resource.ResourceID("22222222-2222-4222-8222-222222222222")
	targets := []resource.Target{
		{},
		{AccountID: accountID, ResourceID: resourceID, Kind: resource.ResourceKindMailbox, View: resource.MailboxViewRecipient, Owner: "owner@example.com"},
		{AccountID: accountID, ResourceID: resourceID, Kind: resource.ResourceKindMailbox, View: resource.MailboxViewOwner},
		{AccountID: accountID, ResourceID: resourceID, Kind: resource.ResourceKindMountedCalendar, View: resource.MailboxViewRecipient, Owner: "owner@example.com"},
	}
	if _, err := SelectUserRoot(nil, targets[0]); err == nil {
		t.Fatal("SelectUserRoot(nil) error = nil")
	}
	for _, target := range targets {
		if _, err := SelectUserRoot(client, target); err == nil {
			t.Fatalf("SelectUserRoot() accepted invalid target: %+v", target)
		}
	}
	if calls.Load() != 0 {
		t.Fatalf("HTTP calls = %d, want zero", calls.Load())
	}
}

// targetTestRewriteMiddleware rewrites Graph hosts after the production Graph
// middleware has replaced the generated Me sentinel with `/me`.
type targetTestRewriteMiddleware struct {
	baseURL string
}

// Intercept changes only scheme and host, preserving the SDK's escaped path.
func (middleware *targetTestRewriteMiddleware) Intercept(pipeline kiotahttp.Pipeline, middlewareIndex int, request *http.Request) (*http.Response, error) {
	request.URL.Scheme = "http"
	request.URL.Host = middleware.baseURL[len("http://"):]
	return pipeline.Next(request, middlewareIndex)
}

// newTargetTestGraphClient constructs the real generated SDK over a local
// anonymous transport and returns its server for cleanup.
func newTargetTestGraphClient(t *testing.T, handler http.Handler) (*msgraphsdk.GraphServiceClient, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(handler)
	middlewares := msgraphcore.GetDefaultMiddlewaresWithOptions(nil)
	middlewares = append(middlewares, &targetTestRewriteMiddleware{baseURL: server.URL})
	httpClient := kiotahttp.GetDefaultClient(middlewares...)
	adapter, err := msgraphsdk.NewGraphRequestAdapterWithParseNodeFactoryAndSerializationWriterFactoryAndHttpClient(
		&kiotaauth.AnonymousAuthenticationProvider{}, nil, nil, httpClient,
	)
	if err != nil {
		server.Close()
		t.Fatalf("create Graph request adapter: %v", err)
	}
	return msgraphsdk.NewGraphServiceClient(adapter), server
}
