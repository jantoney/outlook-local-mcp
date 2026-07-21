package auth

import (
	"context"
	"sync"
)

// auditOutcomeRecorder synchronizes one request's evolving audit outcome.
type auditOutcomeRecorder struct {
	mu      sync.Mutex
	outcome string
}

// auditOutcomeKeyType prevents collisions with context keys owned by callers.
type auditOutcomeKeyType struct{}

var auditOutcomeKey auditOutcomeKeyType

// WithAuditOutcomeRecorder returns a derived context that lets a handler
// classify a non-error result more precisely than ordinary success.
func WithAuditOutcomeRecorder(ctx context.Context) context.Context {
	return context.WithValue(ctx, auditOutcomeKey, &auditOutcomeRecorder{})
}

// RecordAuditOutcome records a request-local outcome such as partial_success.
// It is a no-op when audit middleware did not install a recorder.
func RecordAuditOutcome(ctx context.Context, outcome string) {
	recorder, ok := ctx.Value(auditOutcomeKey).(*auditOutcomeRecorder)
	if !ok || recorder == nil {
		return
	}
	recorder.mu.Lock()
	recorder.outcome = outcome
	recorder.mu.Unlock()
}

// AuditOutcomeFromContext returns the handler-recorded outcome, or false when
// normal success/error classification should apply.
func AuditOutcomeFromContext(ctx context.Context) (string, bool) {
	recorder, ok := ctx.Value(auditOutcomeKey).(*auditOutcomeRecorder)
	if !ok || recorder == nil {
		return "", false
	}
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	return recorder.outcome, recorder.outcome != ""
}
