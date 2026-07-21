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

// supportsOwnerRead reports whether this issue's read surface supports target.
// Mounted-calendar reads are enabled separately by issue #9.
func (target calendarReadTarget) supportsOwnerRead() bool {
	return !target.isShared() || target.target.Kind == resource.ResourceKindOwnerPrimaryCalendar
}
