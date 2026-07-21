package tools

import (
	"context"
	"fmt"

	"github.com/desek/outlook-local-mcp/internal/graph"
	"github.com/desek/outlook-local-mcp/internal/resource"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/microsoftgraph/msgraph-sdk-go/models"
	"github.com/microsoftgraph/msgraph-sdk-go/users"
)

// getCalendarEvent reads one event from the exact routed root and mounted ID.
func getCalendarEvent(ctx context.Context, target calendarReadTarget, eventID string, selectFields []string) (models.Eventable, error) {
	if target.isMounted() {
		cfg := &users.ItemCalendarsItemEventsEventItemRequestBuilderGetRequestConfiguration{
			QueryParameters: &users.ItemCalendarsItemEventsEventItemRequestBuilderGetQueryParameters{Select: selectFields},
		}
		return target.root.Calendars().ByCalendarId(target.target.MountedCalendarID).Events().ByEventId(eventID).Get(ctx, cfg)
	}
	cfg := &users.ItemEventsEventItemRequestBuilderGetRequestConfiguration{
		QueryParameters: &users.ItemEventsEventItemRequestBuilderGetQueryParameters{Select: selectFields},
	}
	return target.root.Events().ByEventId(eventID).Get(ctx, cfg)
}

// createCalendarEvent posts one event to the exact own or mounted collection.
func createCalendarEvent(ctx context.Context, target calendarReadTarget, ownCalendarID string, event models.Eventable) (models.Eventable, error) {
	if target.isMounted() {
		config := &users.ItemCalendarsItemEventsRequestBuilderPostRequestConfiguration{Options: graph.NoRetryRequestOptions()}
		return target.root.Calendars().ByCalendarId(target.target.MountedCalendarID).Events().Post(ctx, event, config)
	}
	if ownCalendarID != "" {
		config := &users.ItemCalendarsItemEventsRequestBuilderPostRequestConfiguration{Options: graph.NoRetryRequestOptions()}
		return target.root.Calendars().ByCalendarId(ownCalendarID).Events().Post(ctx, event, config)
	}
	config := &users.ItemEventsRequestBuilderPostRequestConfiguration{Options: graph.NoRetryRequestOptions()}
	return target.root.Events().Post(ctx, event, config)
}

// patchCalendarEvent updates one event without changing its authorized route.
func patchCalendarEvent(ctx context.Context, target calendarReadTarget, eventID string, event models.Eventable) (models.Eventable, error) {
	if target.isMounted() {
		return target.root.Calendars().ByCalendarId(target.target.MountedCalendarID).Events().ByEventId(eventID).Patch(ctx, event, nil)
	}
	return target.root.Events().ByEventId(eventID).Patch(ctx, event, nil)
}

// deleteCalendarEvent deletes one event without changing its authorized route.
func deleteCalendarEvent(ctx context.Context, target calendarReadTarget, eventID string) error {
	if target.isMounted() {
		config := &users.ItemCalendarsItemEventsEventItemRequestBuilderDeleteRequestConfiguration{Options: graph.NoRetryRequestOptions()}
		return target.root.Calendars().ByCalendarId(target.target.MountedCalendarID).Events().ByEventId(eventID).Delete(ctx, config)
	}
	config := &users.ItemEventsEventItemRequestBuilderDeleteRequestConfiguration{Options: graph.NoRetryRequestOptions()}
	return target.root.Events().ByEventId(eventID).Delete(ctx, config)
}

// requireMountedNonMeeting accepts only events Graph explicitly classifies as
// having neither attendees nor online-meeting capability.
func requireMountedNonMeeting(event models.Eventable) error {
	if event == nil {
		return fmt.Errorf("event preflight returned no event")
	}
	if len(event.GetAttendees()) != 0 {
		return fmt.Errorf("mounted calendar management supports only events without attendees or online-meeting capability; no mutation was started")
	}
	online := event.GetIsOnlineMeeting()
	if online == nil {
		return fmt.Errorf("mounted event classification is unavailable because isOnlineMeeting was not returned; no mutation was started")
	}
	if *online {
		return fmt.Errorf("mounted calendar management does not support online meetings; no mutation was started; manage this meeting from the organizer's own calendar")
	}
	return nil
}

// rejectMountedOnlineMeetingInput rejects requests that would create durable
// online-meeting capability through the intentionally narrow mounted route.
func rejectMountedOnlineMeetingInput(target calendarReadTarget, request mcp.CallToolRequest) error {
	if target.isShared() && request.GetBool("is_online_meeting", false) {
		return fmt.Errorf("mounted calendar management cannot enable online meetings; no Graph request was started; use the organizer's own calendar")
	}
	return nil
}

// appendSharedEventReference adds renewed provenance to a write confirmation.
func appendSharedEventReference(response string, target calendarReadTarget, codec *resource.ReferenceCodec, eventID string) (string, error) {
	if !target.isShared() {
		return response, nil
	}
	if codec == nil {
		return "", fmt.Errorf("shared resource reference signing is unavailable")
	}
	reference, err := signCalendarEvent(codec, target.target, eventID)
	if err != nil {
		return "", err
	}
	return response + "\nResource Ref: " + reference, nil
}

// revalidateCalendarTarget checks current authority before a committed write.
func revalidateCalendarTarget(ctx context.Context) error {
	routed, ok := graph.RoutedTargetFromContext(ctx)
	if !ok {
		return fmt.Errorf("calendar target context is unavailable")
	}
	return routed.Revalidate()
}

// rejectSharedMeetingRoute prevents undeclared shared inputs from silently
// falling back to an own-calendar meeting action.
func rejectSharedMeetingRoute(request mcp.CallToolRequest) error {
	if request.GetString("shared_resource", "") != "" || request.GetString("resource_ref", "") != "" {
		return fmt.Errorf("shared meeting actions are not supported; use meeting operations only on the signed-in account's own calendar")
	}
	return nil
}
