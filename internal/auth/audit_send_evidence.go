package auth

import (
	"context"
	"sync"
)

// AuditSendEvidence contains only counts and keyed fingerprints from a shared
// send review. It intentionally excludes raw presentation data.
type AuditSendEvidence struct {
	// RecipientCount is the total To, Cc, and Bcc multiset size.
	RecipientCount int
	// AttachmentCount is the complete paged attachment-set size.
	AttachmentCount int
	// ReferenceFingerprint correlates the exact signed draft reference.
	ReferenceFingerprint string
	// ChangeKeyFingerprint correlates the reviewed Graph version.
	ChangeKeyFingerprint string
	// SubjectFingerprint correlates the reviewed subject without storing it.
	SubjectFingerprint string
	// RecipientFingerprint correlates the field-separated recipient multisets.
	RecipientFingerprint string
	// AttachmentFingerprint correlates attachment identities and metadata.
	AttachmentFingerprint string
	// SendAttempt reports whether the one Graph POST stage began.
	SendAttempt bool
}

// auditSendEvidenceRecorder synchronizes sanitized evidence for one request.
type auditSendEvidenceRecorder struct {
	mu       sync.Mutex
	evidence AuditSendEvidence
	set      bool
}

// auditSendEvidenceKeyType prevents collisions with caller-owned context keys.
type auditSendEvidenceKeyType struct{}

var auditSendEvidenceKey auditSendEvidenceKeyType

// WithAuditSendEvidenceRecorder returns a context that accepts request-local
// shared-send audit evidence.
func WithAuditSendEvidenceRecorder(ctx context.Context) context.Context {
	return context.WithValue(ctx, auditSendEvidenceKey, &auditSendEvidenceRecorder{})
}

// RecordAuditSendEvidence records sanitized review evidence. It is a no-op
// when audit middleware did not install a recorder.
func RecordAuditSendEvidence(ctx context.Context, evidence AuditSendEvidence) {
	recorder, ok := ctx.Value(auditSendEvidenceKey).(*auditSendEvidenceRecorder)
	if !ok || recorder == nil {
		return
	}
	recorder.mu.Lock()
	recorder.evidence = evidence
	recorder.set = true
	recorder.mu.Unlock()
}

// MarkAuditSendAttempt records that the single send POST stage began.
func MarkAuditSendAttempt(ctx context.Context) {
	recorder, ok := ctx.Value(auditSendEvidenceKey).(*auditSendEvidenceRecorder)
	if !ok || recorder == nil {
		return
	}
	recorder.mu.Lock()
	recorder.evidence.SendAttempt = true
	recorder.set = true
	recorder.mu.Unlock()
}

// AuditSendEvidenceFromContext returns sanitized send evidence, or false when
// the request was not a reviewed shared send.
func AuditSendEvidenceFromContext(ctx context.Context) (AuditSendEvidence, bool) {
	recorder, ok := ctx.Value(auditSendEvidenceKey).(*auditSendEvidenceRecorder)
	if !ok || recorder == nil {
		return AuditSendEvidence{}, false
	}
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	return recorder.evidence, recorder.set
}
