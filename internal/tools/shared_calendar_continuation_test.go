package tools

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/microsoftgraph/msgraph-sdk-go/models"
)

// TestValidateCalendarContinuationPinsExactCollection verifies calendar next
// links can change only their query on the exact authorized public Graph route.
func TestValidateCalendarContinuationPinsExactCollection(t *testing.T) {
	tests := []struct {
		name       string
		link       string
		collection string
		ok         bool
	}{
		{name: "owner primary", link: "https://graph.microsoft.com/v1.0/users/owner%2Bops%40example.com/calendarView?$skiptoken=next", collection: "/v1.0/users/owner+ops@example.com/calendarView", ok: true},
		{name: "mounted", link: "https://graph.microsoft.com/v1.0/me/calendars/mounted%2B1/calendarView?$skiptoken=next", collection: "/v1.0/me/calendars/mounted+1/calendarView", ok: true},
		{name: "discovery", link: "https://graph.microsoft.com/v1.0/me/calendars?$skiptoken=next", collection: "/v1.0/me/calendars", ok: true},
		{name: "hostile host", link: "https://evil.example/v1.0/me/calendars?$skiptoken=next", collection: "/v1.0/me/calendars"},
		{name: "host suffix", link: "https://graph.microsoft.com.evil.example/v1.0/me/calendars?$skiptoken=next", collection: "/v1.0/me/calendars"},
		{name: "explicit port", link: "https://graph.microsoft.com:443/v1.0/me/calendars?$skiptoken=next", collection: "/v1.0/me/calendars"},
		{name: "owner to me", link: "https://graph.microsoft.com/v1.0/me/calendarView?$skiptoken=next", collection: "/v1.0/users/owner@example.com/calendarView"},
		{name: "mounted switch", link: "https://graph.microsoft.com/v1.0/me/calendars/other/calendarView?$skiptoken=next", collection: "/v1.0/me/calendars/mounted/calendarView"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateCalendarContinuation(test.link, test.collection)
			if test.ok && err != nil {
				t.Fatalf("validateCalendarContinuation() error = %v", err)
			}
			if !test.ok && err == nil {
				t.Fatal("validateCalendarContinuation() succeeded")
			}
		})
	}
}

// TestIteratePinnedSharedEventsRevalidatesEveryPage verifies a valid first
// continuation does not authorize a hostile link returned by page two.
func TestIteratePinnedSharedEventsRevalidatesEveryPage(t *testing.T) {
	var calls int
	client, server := newRealSDKGraphClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"value":[{"id":"event-2"}],"@odata.nextLink":"https://evil.example/v1.0/users/owner%40example.com/calendarView?$skiptoken=page-3"}`))
	}))
	defer server.Close()

	first := models.NewEventCollectionResponse()
	event := models.NewEvent()
	eventID := "event-1"
	event.SetId(&eventID)
	first.SetValue([]models.Eventable{event})
	next := "https://graph.microsoft.com/v1.0/users/owner%40example.com/calendarView?$skiptoken=page-2"
	first.SetOdataNextLink(&next)

	var visited int
	err := iteratePinnedSharedEvents(context.Background(), first, client.GetAdapter(),
		"/v1.0/users/owner@example.com/calendarView", nil, func(models.Eventable) bool {
			visited++
			return true
		})
	if err == nil || !strings.Contains(err.Error(), "continuation") {
		t.Fatalf("iteratePinnedSharedEvents() error = %v, want continuation denial", err)
	}
	if calls != 1 || visited != 2 {
		t.Fatalf("Graph calls = %d, visited = %d; want one accepted continuation and two events", calls, visited)
	}
}
