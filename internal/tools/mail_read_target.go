package tools

import (
	"context"
	"fmt"

	"github.com/desek/outlook-local-mcp/internal/graph"
	"github.com/desek/outlook-local-mcp/internal/resource"
	graphusers "github.com/microsoftgraph/msgraph-sdk-go/users"
)

// mailReadTarget contains the typed request root and immutable target selected
// by server middleware for one own or shared-mail read.
type mailReadTarget struct {
	root   *graphusers.UserItemRequestBuilder
	target resource.Target
	claims *resource.ReferenceClaims
}

// mailTargetFromContext returns the guarded routed target. Direct unit calls
// without routing context retain the historical signed-in /me root.
func mailTargetFromContext(ctx context.Context) (mailReadTarget, error) {
	if routed, ok := graph.RoutedTargetFromContext(ctx); ok {
		if routed.Root == nil {
			return mailReadTarget{}, fmt.Errorf("mail target has no Graph root")
		}
		return mailReadTarget{root: routed.Root, target: routed.Target, claims: routed.Claims}, nil
	}
	client, err := GraphClient(ctx)
	if err != nil {
		return mailReadTarget{}, fmt.Errorf("no account selected")
	}
	return mailReadTarget{root: client.Me()}, nil
}

// isShared reports whether middleware selected a configured shared mailbox.
func (target mailReadTarget) isShared() bool { return target.target.Alias != "" }

// folderID returns an own raw folder ID or a verified optional shared folder
// ID. An empty shared folder reference selects the owner's all-messages route.
func (target mailReadTarget) folderID(requestID string) (string, error) {
	if !target.isShared() {
		return requestID, nil
	}
	if requestID != "" {
		return "", fmt.Errorf("shared mailbox folder selection requires folder_ref and does not accept folder_id")
	}
	if target.claims == nil {
		return "", nil
	}
	if target.claims.ItemKind != resource.ItemKindMailFolder || len(target.claims.GraphIDChain) != 1 {
		return "", fmt.Errorf("shared mailbox folder selection requires a verified folder_ref")
	}
	return target.claims.GraphIDChain[0].ID, nil
}

// messageID returns an own raw message ID or the verified shared message ID.
func (target mailReadTarget) messageID(requestID string) (string, error) {
	if !target.isShared() {
		if requestID == "" {
			return "", fmt.Errorf("missing required parameter: message_id. Tip: Use mail_list_messages or mail_search_messages to find the message ID.")
		}
		return requestID, nil
	}
	if requestID != "" {
		return "", fmt.Errorf("shared mailbox follow-up requires resource_ref and does not accept message_id")
	}
	if target.claims == nil || target.claims.ItemKind != resource.ItemKindMessage || len(target.claims.GraphIDChain) != 1 {
		return "", fmt.Errorf("shared mailbox follow-up requires a verified message resource_ref")
	}
	return target.claims.GraphIDChain[0].ID, nil
}

// referencedMessageID returns a verified message ID for new operations whose
// contract requires references for both own and shared targets.
func (target mailReadTarget) referencedMessageID() (string, error) {
	if target.claims == nil || target.claims.ItemKind != resource.ItemKindMessage || len(target.claims.GraphIDChain) != 1 {
		return "", fmt.Errorf("operation requires a verified message_ref for the resolved mailbox")
	}
	return target.claims.GraphIDChain[0].ID, nil
}

// draftID returns an own raw draft ID or the verified shared draft ID.
func (target mailReadTarget) draftID(requestID string) (string, error) {
	if !target.isShared() {
		if requestID == "" {
			return "", fmt.Errorf("missing required parameter: message_id")
		}
		return requestID, nil
	}
	if requestID != "" {
		return "", fmt.Errorf("shared mailbox draft mutation requires draft_ref and does not accept message_id")
	}
	if target.claims == nil || target.claims.ItemKind != resource.ItemKindDraft || len(target.claims.GraphIDChain) != 1 {
		return "", fmt.Errorf("shared mailbox draft mutation requires a verified draft_ref")
	}
	return target.claims.GraphIDChain[0].ID, nil
}

// attachmentID returns an own raw attachment ID or verifies a shared
// attachment reference whose parent message matches the guarded message claim.
func (target mailReadTarget) attachmentID(requestID, reference string, codec *resource.ReferenceCodec) (string, error) {
	if !target.isShared() {
		if requestID == "" {
			return "", fmt.Errorf("missing required parameter: attachment_id")
		}
		return requestID, nil
	}
	if requestID != "" {
		return "", fmt.Errorf("shared mailbox attachment download requires attachment_ref and does not accept attachment_id")
	}
	if reference == "" || codec == nil {
		return "", fmt.Errorf("shared mailbox attachment download requires a verified attachment_ref")
	}
	claims, err := codec.Verify(reference)
	if err != nil {
		return "", err
	}
	if claims.AccountID != target.target.AccountID || claims.ResourceID != target.target.ResourceID ||
		claims.ResourceKind != target.target.Kind || claims.MailboxView != target.target.View ||
		claims.ItemKind != resource.ItemKindAttachment || len(claims.GraphIDChain) != 2 {
		return "", fmt.Errorf("attachment reference does not match the resolved shared mailbox")
	}
	if target.claims == nil || len(target.claims.GraphIDChain) != 1 || claims.GraphIDChain[0].ID != target.claims.GraphIDChain[0].ID {
		return "", fmt.Errorf("attachment reference does not match the verified parent message")
	}
	return claims.GraphIDChain[1].ID, nil
}

// graphError returns a redacted Graph error without route fallback.
func (target mailReadTarget) graphError(err error) string {
	if !target.isShared() {
		return graph.RedactGraphError(err)
	}
	return fmt.Sprintf("shared mailbox owner-view request failed without fallback: %s", graph.RedactGraphError(err))
}
