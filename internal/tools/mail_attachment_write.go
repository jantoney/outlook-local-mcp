package tools

import (
	"context"
	"fmt"

	"github.com/desek/outlook-local-mcp/internal/graph"
	"github.com/desek/outlook-local-mcp/internal/resource"
)

// attachmentMutationError reports ambiguity without retrying a completed or
// possibly completed direct attachment mutation.
func attachmentMutationError(target mailReadTarget, err error) string {
	status := graph.ExtractHTTPStatus(err)
	if status == 0 || status >= 500 {
		return "attachment upload outcome is uncertain; do not repeat the upload until the draft is inspected. Detail: " + target.graphError(err)
	}
	return target.graphError(err)
}

// sharedAttachmentConfirmation adds renewed draft provenance and immutable
// target display metadata to a verified shared attachment outcome.
func sharedAttachmentConfirmation(response string, target mailReadTarget, codec *resource.ReferenceCodec, draftID string) (string, error) {
	if !target.isShared() {
		return response, nil
	}
	referenced, err := appendSharedDraftReference(response, target, codec, draftID)
	if err != nil {
		return "", err
	}
	return referenced + fmt.Sprintf("\nShared Target: %s (%s)", target.target.Alias, target.target.Owner), nil
}

// sharedAttachmentPartial returns recovery-safe metadata for a completed
// shared Graph mutation whose verification or provenance tail was incomplete.
func sharedAttachmentPartial(ctx context.Context, target mailReadTarget, codec *resource.ReferenceCodec, draftID, attachmentID, name string, size int64, contentType, detail string) string {
	recordDraftPartialSuccess(ctx)
	response := fmt.Sprintf("PARTIAL SUCCESS: attachment upload completed, but verification or provenance could not be confirmed.\nAttachment: %s (%d bytes, %s)\nDraft ID: %s", name, size, contentType, draftID)
	if attachmentID != "" {
		response += "\nAttachment ID: " + attachmentID
	}
	if referenced, err := sharedAttachmentConfirmation("", target, codec, draftID); err == nil {
		response += referenced
	} else {
		response += "\nDraft Ref: unavailable (" + err.Error() + ")"
		response += fmt.Sprintf("\nShared Target: %s (%s)", target.target.Alias, target.target.Owner)
	}
	response += "\nCompletion detail: " + detail
	response += "\nDo not repeat the upload until the existing draft's attachments are inspected."
	return response
}
