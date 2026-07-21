package graph

import (
	"net/http"
	"time"

	abstractions "github.com/microsoft/kiota-abstractions-go"
	nethttplibrary "github.com/microsoft/kiota-http-go"
)

// NoRetryRequestOptions returns a fresh per-request Kiota option slice that
// suppresses middleware retries for mutations whose ambiguous first response
// cannot safely be repeated. It accepts no parameters, performs no I/O, and
// cannot fail; the caller attaches the result to a request configuration.
func NoRetryRequestOptions() []abstractions.RequestOption {
	return []abstractions.RequestOption{&nethttplibrary.RetryHandlerOptions{
		ShouldRetry: func(time.Duration, int, *http.Request, *http.Response) bool { return false },
	}}
}
