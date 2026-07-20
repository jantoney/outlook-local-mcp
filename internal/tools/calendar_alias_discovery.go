package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/desek/outlook-local-mcp/internal/auth"
	"github.com/desek/outlook-local-mcp/internal/graph"
	msgraphcore "github.com/microsoftgraph/msgraph-sdk-go-core"
	"github.com/microsoftgraph/msgraph-sdk-go/models"
)

// mountedCalendarCandidate is one recipient-view calendar returned by Graph
// whose owner address exactly matches the configured owner locator.
type mountedCalendarCandidate struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Owner   string `json:"owner"`
	CanEdit bool   `json:"can_edit"`
}

// discoverMountedCalendarCandidates lists every /me/calendars page and returns
// only calendars whose Graph owner address matches owner. It never infers a
// target by name and returns an error for disconnected accounts.
func discoverMountedCalendarCandidates(ctx context.Context, entry *auth.AccountEntry, owner string, retryCfg graph.RetryConfig, timeout time.Duration) ([]mountedCalendarCandidate, error) {
	if entry == nil || entry.Client == nil || !entry.Authenticated {
		return nil, fmt.Errorf("account must be connected before mounted-calendar discovery")
	}
	timeoutCtx, cancel := graph.WithTimeout(ctx, timeout)
	defer cancel()

	var response models.CalendarCollectionResponseable
	err := graph.RetryGraphCall(ctx, retryCfg, func() error {
		var graphErr error
		response, graphErr = entry.Client.Me().Calendars().Get(timeoutCtx, nil)
		return graphErr
	})
	if err != nil {
		return nil, fmt.Errorf("discover mounted calendars: %w", err)
	}
	iterator, err := msgraphcore.NewPageIterator[models.Calendarable](
		response, entry.Client.GetAdapter(), models.CreateCalendarCollectionResponseFromDiscriminatorValue,
	)
	if err != nil {
		return nil, fmt.Errorf("create calendar discovery page iterator: %w", err)
	}
	candidates := make([]mountedCalendarCandidate, 0)
	if err := iterator.Iterate(timeoutCtx, func(calendar models.Calendarable) bool {
		calendarOwner := ""
		if value := calendar.GetOwner(); value != nil {
			calendarOwner = graph.SafeStr(value.GetAddress())
		}
		if strings.EqualFold(calendarOwner, owner) {
			candidates = append(candidates, mountedCalendarCandidate{
				ID: graph.SafeStr(calendar.GetId()), Name: graph.SafeStr(calendar.GetName()),
				Owner: calendarOwner, CanEdit: graph.SafeBool(calendar.GetCanEdit()),
			})
		}
		return true
	}); err != nil {
		return nil, fmt.Errorf("iterate mounted calendars: %w", err)
	}
	return candidates, nil
}
