package tools

import (
	"fmt"

	"github.com/desek/outlook-local-mcp/internal/graph"
)

// calendarMutationOutcomeUncertain reports whether an attempted calendar
// mutation may have reached Graph despite its error. Transport/status-zero,
// timeout, throttling, and server errors are ambiguous; definite 4xx are not.
func calendarMutationOutcomeUncertain(err error) bool {
	status := graph.ExtractHTTPStatus(err)
	return err != nil && (graph.IsTimeoutError(err) || status == 0 || status == 429 || status >= 500)
}

// ambiguousCalendarMutationError builds safe recovery guidance for action.
// reconcile tells the caller what authoritative state to inspect. The return
// value is redacted and the function performs no I/O or state changes.
func ambiguousCalendarMutationError(action, reconcile string, err error) string {
	return fmt.Sprintf("%s outcome is uncertain; do not retry blindly. %s Detail: %s", action, reconcile, graph.RedactGraphError(err))
}
