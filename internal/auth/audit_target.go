package auth

import (
	"context"
	"sync"

	"github.com/desek/outlook-local-mcp/internal/resource"
)

// AuditTarget describes the exact local resource decision made by a handler.
// It lets outer audit middleware report target-local policy and compatibility
// without re-resolving mutable aliases after the operation completes.
type AuditTarget struct {
	// Alias is the account-scoped human selector used by the request.
	Alias string
	// ResourceID is the immutable target identity.
	ResourceID resource.ResourceID
	// ResourceKind is the immutable mailbox or calendar kind.
	ResourceKind resource.ResourceKind
	// MailboxView is the immutable Graph identifier namespace.
	MailboxView resource.MailboxView
	// MailPolicy is the exact selected shared-mail action matrix.
	MailPolicy MailActionPolicy
	// Compatibility states the local token-context compatibility decision.
	Compatibility string
}

type auditTargetRecorder struct {
	mu     sync.Mutex
	target AuditTarget
	set    bool
}

type auditTargetKeyType struct{}

var auditTargetKey auditTargetKeyType

// WithAuditTargetRecorder returns a derived context containing a request-local
// mutable recorder. Outer middleware creates it before invoking handlers.
func WithAuditTargetRecorder(ctx context.Context) context.Context {
	return context.WithValue(ctx, auditTargetKey, &auditTargetRecorder{})
}

// RecordAuditTarget records the handler's exact selected target snapshot. It
// is a no-op when the audit recorder middleware is absent.
func RecordAuditTarget(ctx context.Context, target AuditTarget) {
	recorder, ok := ctx.Value(auditTargetKey).(*auditTargetRecorder)
	if !ok || recorder == nil {
		return
	}
	recorder.mu.Lock()
	recorder.target = target
	recorder.set = true
	recorder.mu.Unlock()
}

// AuditTargetFromContext returns the target snapshot recorded by the handler,
// or false when no target-specific decision occurred.
func AuditTargetFromContext(ctx context.Context) (AuditTarget, bool) {
	recorder, ok := ctx.Value(auditTargetKey).(*auditTargetRecorder)
	if !ok || recorder == nil {
		return AuditTarget{}, false
	}
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	return recorder.target, recorder.set
}
