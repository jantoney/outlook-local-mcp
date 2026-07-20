package graph

import (
	"context"

	"github.com/desek/outlook-local-mcp/internal/resource"
	graphusers "github.com/microsoftgraph/msgraph-sdk-go/users"
)

// RoutedTarget is the authorized immutable target plus its one typed SDK user
// root and optional verified item claims. Creating it performs no HTTP traffic.
type RoutedTarget struct {
	// Target is the exact local resource authorization snapshot.
	Target resource.Target
	// Root is either the generated Me or Users.ByUserId request builder.
	Root *graphusers.UserItemRequestBuilder
	// Claims is verified signed provenance, or nil for collection operations.
	Claims *resource.ReferenceClaims
}

type routedTargetKeyType struct{}

var routedTargetKey routedTargetKeyType

// WithRoutedTarget returns a derived context containing a fully authorized and
// routed target. Callers must not use it until every guard has succeeded.
func WithRoutedTarget(ctx context.Context, target RoutedTarget) context.Context {
	return context.WithValue(ctx, routedTargetKey, target)
}

// RoutedTargetFromContext returns the request target produced by middleware,
// or false when resolution or routing did not complete.
func RoutedTargetFromContext(ctx context.Context) (RoutedTarget, bool) {
	if ctx == nil {
		return RoutedTarget{}, false
	}
	target, ok := ctx.Value(routedTargetKey).(RoutedTarget)
	return target, ok
}
