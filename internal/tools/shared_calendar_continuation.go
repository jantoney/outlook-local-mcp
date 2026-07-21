package tools

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	abstractions "github.com/microsoft/kiota-abstractions-go"
	"github.com/microsoft/kiota-abstractions-go/serialization"
	"github.com/microsoftgraph/msgraph-sdk-go/models"
)

// sharedCalendarViewPath returns the decoded Graph collection bound to one
// authorized shared calendar target.
func sharedCalendarViewPath(target calendarReadTarget) string {
	if target.isMounted() {
		return "/v1.0/me/calendars/" + target.target.MountedCalendarID + "/calendarView"
	}
	return "/v1.0/users/" + target.target.Owner + "/calendarView"
}

// validateCalendarContinuation requires one Graph next link to retain the
// public HTTPS origin and exact decoded calendar collection selected locally.
// It performs no I/O and rejects userinfo, ports, fragments, and route changes.
func validateCalendarContinuation(nextLink, expectedCollection string) error {
	parsed, err := url.Parse(nextLink)
	if err != nil || parsed.User != nil || parsed.Fragment != "" ||
		!strings.EqualFold(parsed.Scheme, "https") ||
		!strings.EqualFold(parsed.Host, publicGraphOrigin) {
		return fmt.Errorf("calendar pagination continuation must use the public HTTPS Graph origin")
	}
	decodedPath, err := url.PathUnescape(parsed.EscapedPath())
	if err != nil || decodedPath != expectedCollection {
		return fmt.Errorf("calendar pagination continuation changed the authorized collection")
	}
	return nil
}

// iteratePinnedSharedEvents visits every event page while validating each
// continuation before its Graph GET. The callback controls result limits.
func iteratePinnedSharedEvents(
	ctx context.Context,
	first models.EventCollectionResponseable,
	adapter abstractions.RequestAdapter,
	collection string,
	headers *abstractions.RequestHeaders,
	callback func(models.Eventable) bool,
) error {
	page := first
	for {
		for _, event := range page.GetValue() {
			if !callback(event) {
				return nil
			}
		}
		nextLink := page.GetOdataNextLink()
		if nextLink == nil || *nextLink == "" {
			return nil
		}
		if err := validateCalendarContinuation(*nextLink, collection); err != nil {
			return err
		}
		response, err := fetchPinnedCalendarPage(ctx, adapter, *nextLink, headers,
			models.CreateEventCollectionResponseFromDiscriminatorValue)
		if err != nil {
			return err
		}
		var ok bool
		page, ok = response.(models.EventCollectionResponseable)
		if !ok {
			return fmt.Errorf("calendar continuation returned an invalid event collection")
		}
	}
}

// iteratePinnedMountedCalendars visits every mounted-discovery page while
// requiring the continuation to remain on the `/me/calendars` collection.
func iteratePinnedMountedCalendars(
	ctx context.Context,
	first models.CalendarCollectionResponseable,
	adapter abstractions.RequestAdapter,
	callback func(models.Calendarable) bool,
) error {
	page := first
	for {
		for _, calendar := range page.GetValue() {
			if !callback(calendar) {
				return nil
			}
		}
		nextLink := page.GetOdataNextLink()
		if nextLink == nil || *nextLink == "" {
			return nil
		}
		if err := validateCalendarContinuation(*nextLink, "/v1.0/me/calendars"); err != nil {
			return err
		}
		response, err := fetchPinnedCalendarPage(ctx, adapter, *nextLink, nil,
			models.CreateCalendarCollectionResponseFromDiscriminatorValue)
		if err != nil {
			return err
		}
		var ok bool
		page, ok = response.(models.CalendarCollectionResponseable)
		if !ok {
			return fmt.Errorf("calendar continuation returned an invalid calendar collection")
		}
	}
}

// fetchPinnedCalendarPage sends one already validated continuation request and
// returns its parsed collection. It performs exactly one Graph GET.
func fetchPinnedCalendarPage(
	ctx context.Context,
	adapter abstractions.RequestAdapter,
	nextLink string,
	headers *abstractions.RequestHeaders,
	constructor serialization.ParsableFactory,
) (serialization.Parsable, error) {
	parsed, err := url.Parse(nextLink)
	if err != nil {
		return nil, fmt.Errorf("parse calendar pagination continuation: %w", err)
	}
	requestInfo := abstractions.NewRequestInformation()
	requestInfo.Method = abstractions.GET
	requestInfo.SetUri(*parsed)
	if headers != nil {
		requestInfo.Headers.AddAll(headers)
	}
	response, err := adapter.Send(ctx, requestInfo, constructor, nil)
	if err != nil {
		return nil, fmt.Errorf("fetch calendar pagination continuation: %w", err)
	}
	return response, nil
}
