package tools

import (
	"context"
	"fmt"

	"github.com/desek/outlook-local-mcp/internal/graph"
	"github.com/desek/outlook-local-mcp/internal/resource"
	graphusers "github.com/microsoftgraph/msgraph-sdk-go/users"
)

// calendarReadTarget contains the typed request root and immutable target
// selected by server middleware for one calendar read.
type calendarReadTarget struct {
	root   *graphusers.UserItemRequestBuilder
	target resource.Target
	claims *resource.ReferenceClaims
}

// calendarTargetFromContext returns the guarded routed target. Direct unit
// calls without routing context retain the historical own-calendar root using
// the Graph client already injected into context.
func calendarTargetFromContext(ctx context.Context) (calendarReadTarget, error) {
	if routed, ok := graph.RoutedTargetFromContext(ctx); ok {
		if routed.Root == nil {
			return calendarReadTarget{}, fmt.Errorf("calendar target has no Graph root")
		}
		return calendarReadTarget{root: routed.Root, target: routed.Target, claims: routed.Claims}, nil
	}
	client, err := GraphClient(ctx)
	if err != nil {
		return calendarReadTarget{}, fmt.Errorf("no account selected")
	}
	return calendarReadTarget{root: client.Me()}, nil
}

// isShared reports whether middleware selected a configured calendar alias.
func (target calendarReadTarget) isShared() bool {
	return target.target.Alias != ""
}

// eventID returns the own raw ID or the verified shared event ID. Shared calls
// reject alias-plus-raw-ID addressing even when a handler is invoked directly.
func (target calendarReadTarget) eventID(requestID string) (string, error) {
	if !target.isShared() {
		if requestID == "" {
			return "", fmt.Errorf("missing required parameter: event_id. Tip: Use calendar_list_events or calendar_search_events to find the event ID")
		}
		return requestID, nil
	}
	if requestID != "" {
		return "", fmt.Errorf("shared calendar follow-up requires resource_ref and does not accept event_id")
	}
	if target.claims == nil || target.claims.ItemKind != resource.ItemKindEvent || len(target.claims.GraphIDChain) != 1 {
		return "", fmt.Errorf("shared calendar follow-up requires a verified event resource_ref")
	}
	return target.claims.GraphIDChain[0].ID, nil
}

// supportsSharedRead reports whether the calendar read surface supports target.
func (target calendarReadTarget) supportsSharedRead() bool {
	return !target.isShared() || target.target.Kind == resource.ResourceKindOwnerPrimaryCalendar || target.isMounted()
}

// isMounted reports whether the shared target is one selected recipient-view
// calendar whose configured ID must be applied beneath the /me user root.
func (target calendarReadTarget) isMounted() bool {
	return target.target.Kind == resource.ResourceKindMountedCalendar
}

// graphError returns an actionable mounted-calendar recovery error without
// changing the route or attempting owner/default fallbacks.
func (target calendarReadTarget) graphError(err error) string {
	redacted := graph.RedactGraphError(err)
	if !target.isMounted() {
		return redacted
	}
	return fmt.Sprintf("mounted calendar request failed; use account.reselect_calendar_alias after fresh discovery: %s", redacted)
}

// calendarViewEndpoint returns a route-template label safe for diagnostics.
func (target calendarReadTarget) calendarViewEndpoint() string {
	if target.isMounted() {
		return "GET /me/calendars/{mounted-id}/calendarView"
	}
	if target.target.View == resource.MailboxViewOwner {
		return "GET /users/{owner}/calendarView"
	}
	return "GET /me/calendarView"
}

// eventEndpoint returns a route-template label safe for diagnostics.
func (target calendarReadTarget) eventEndpoint() string {
	if target.isMounted() {
		return "GET /me/calendars/{mounted-id}/events/{event-id}"
	}
	if target.target.View == resource.MailboxViewOwner {
		return "GET /users/{owner}/events/{event-id}"
	}
	return "GET /me/events/{event-id}"
}
