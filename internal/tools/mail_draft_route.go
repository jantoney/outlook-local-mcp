package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/desek/outlook-local-mcp/internal/auth"
	"github.com/desek/outlook-local-mcp/internal/graph"
	"github.com/desek/outlook-local-mcp/internal/resource"
)

// revalidateMailTarget checks current authority before an independently
// committable shared-mail stage. Own direct-handler calls remain compatible.
func revalidateMailTarget(ctx context.Context, target mailReadTarget) error {
	routed, ok := graph.RoutedTargetFromContext(ctx)
	if !ok {
		if target.isShared() {
			return fmt.Errorf("mail target context is unavailable")
		}
		return nil
	}
	return routed.Revalidate()
}

// draftCreateError distinguishes a known Graph rejection from an ambiguous
// transport or server failure. Non-idempotent draft creation is never retried.
func draftCreateError(target mailReadTarget, err error) string {
	status := graph.ExtractHTTPStatus(err)
	if status == 0 || status >= 500 {
		return "draft creation outcome is uncertain; do not repeat the create operation because Graph may already have created a duplicate-prone draft. Inspect Outlook's Drafts folder before taking further action. Detail: " + target.graphError(err)
	}
	return target.graphError(err)
}

// draftWriteConfirmation formats a successful draft write and adds a shared
// target reference when applicable.
func draftWriteConfirmation(action, subject, draftID string, target mailReadTarget, codec *resource.ReferenceCodec) (string, error) {
	if draftID == "" {
		return "", fmt.Errorf("Graph created the draft but returned no draft ID")
	}
	return appendSharedDraftReference(FormatDraftConfirmation(action, subject, draftID), target, codec, draftID)
}

// draftCompletionPartial reports that a non-idempotent create committed but
// its required completion tail could not be confirmed. It never encourages a
// retry, which could create a duplicate draft.
func draftCompletionPartial(kind, subject, draftID, detail string, target mailReadTarget, codec *resource.ReferenceCodec) string {
	response := "PARTIAL SUCCESS: " + kind + " draft created, but required MCP completion could not be confirmed."
	response += "\n" + FormatDraftConfirmation("created", subject, draftID)
	if referenced, err := appendSharedDraftReference("", target, codec, draftID); err == nil {
		response += referenced
	} else if target.isShared() {
		response += "\nDraft Ref: unavailable (" + err.Error() + ")"
	}
	if target.isShared() {
		response += fmt.Sprintf("\nShared Target: %s (%s)", target.target.Alias, target.target.Owner)
	}
	if strings.TrimSpace(detail) != "" {
		response += "\nCompletion detail: " + detail
	}
	response += "\nDo not repeat this create operation; doing so may create a duplicate. Inspect the existing draft by reference if current policy permits, or in Outlook's Drafts folder."
	return response
}

// recordDraftPartialSuccess marks a normal recovery response distinctly in the
// outer audit entry. Direct handler calls without audit middleware are safe.
func recordDraftPartialSuccess(ctx context.Context) {
	auth.RecordAuditOutcome(ctx, "partial_success")
}
