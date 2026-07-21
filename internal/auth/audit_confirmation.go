package auth

import (
	"context"
	"sync"
)

// auditConfirmationRecorder stores the final request-local human confirmation
// state for destructive operations.
type auditConfirmationRecorder struct {
	mu    sync.Mutex
	state string
}

// auditConfirmationKeyType prevents collisions with context keys owned by
// callers or other packages.
type auditConfirmationKeyType struct{}

var auditConfirmationKey auditConfirmationKeyType

// WithAuditConfirmationRecorder returns a context that accepts one evolving
// confirmation state from a handler. It has no external side effects.
func WithAuditConfirmationRecorder(ctx context.Context) context.Context {
	return context.WithValue(ctx, auditConfirmationKey, &auditConfirmationRecorder{})
}

// RecordAuditConfirmation stores the latest confirmation state. It is a no-op
// when audit middleware did not install a recorder.
func RecordAuditConfirmation(ctx context.Context, state string) {
	recorder, ok := ctx.Value(auditConfirmationKey).(*auditConfirmationRecorder)
	if !ok || recorder == nil {
		return
	}
	recorder.mu.Lock()
	recorder.state = state
	recorder.mu.Unlock()
}

// AuditConfirmationFromContext returns the recorded confirmation state, or
// false when the operation did not use a human confirmation gate.
func AuditConfirmationFromContext(ctx context.Context) (string, bool) {
	recorder, ok := ctx.Value(auditConfirmationKey).(*auditConfirmationRecorder)
	if !ok || recorder == nil {
		return "", false
	}
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	return recorder.state, recorder.state != ""
}
