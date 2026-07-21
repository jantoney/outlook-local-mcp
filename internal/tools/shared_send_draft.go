package tools

import (
	"context"
	"fmt"
	"net/url"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/desek/outlook-local-mcp/internal/auth"
	"github.com/desek/outlook-local-mcp/internal/graph"
	"github.com/desek/outlook-local-mcp/internal/logging"
	"github.com/desek/outlook-local-mcp/internal/resource"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/microsoftgraph/msgraph-sdk-go/models"
	"github.com/microsoftgraph/msgraph-sdk-go/users"
)

// sharedSendAttachment is one canonical attachment identity in a complete
// paged shared-draft review snapshot.
type sharedSendAttachment struct {
	ID, Name, ContentType string
	Size                  int32
	Inline                bool
}

// sharedSendSnapshot binds every identity and mutable field reviewed before a
// shared draft is sent. Recipient order within each field is normalized.
type sharedSendSnapshot struct {
	AccountID, ResourceID, DelegateLabel, DelegateUPN string
	Alias, Owner, View, From, ChangeKey, Subject      string
	To, CC, BCC                                       []string
	Attachments                                       []sharedSendAttachment
}

// sharedSendState isolates complete review loading, human elicitation, and the
// one irreversible owner-route send request for fail-closed testing.
type sharedSendState struct {
	load   func(context.Context, mailReadTarget, string) (sharedSendSnapshot, error)
	elicit func(context.Context, mcp.ElicitationRequest) (*mcp.ElicitationResult, error)
	send   func(context.Context, mailReadTarget, string) error
}

// NewHandleSharedSendDraft creates the owner-route shared-send handler. It
// requires complete review evidence, fresh elicitation, exact post-review
// revalidation, and one non-retried send request.
func NewHandleSharedSendDraft(retryCfg graph.RetryConfig, timeout time.Duration, codec *resource.ReferenceCodec) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	state := &sharedSendState{
		load: func(ctx context.Context, target mailReadTarget, draftID string) (sharedSendSnapshot, error) {
			return loadSharedSendSnapshot(ctx, target, retryCfg, timeout, draftID)
		},
		elicit: defaultSendDraftElicit,
		send: func(ctx context.Context, target mailReadTarget, draftID string) error {
			return sendSharedDraftOnce(ctx, target, draftID, timeout)
		},
	}
	return handleSharedSendDraft(state, codec)
}

// sendSharedDraftOnce submits draftID through target's immutable owner route
// within timeout and suppresses Kiota's transparent transient-status retries.
// It returns the single Graph attempt's error and has no local side effects.
func sendSharedDraftOnce(ctx context.Context, target mailReadTarget, draftID string, timeout time.Duration) error {
	timeoutCtx, cancel := graph.WithTimeout(ctx, timeout)
	defer cancel()
	config := &users.ItemMessagesItemSendRequestBuilderPostRequestConfiguration{Options: graph.NoRetryRequestOptions()}
	return target.root.Messages().ByMessageId(draftID).Send().Post(timeoutCtx, config)
}

// handleSharedSendDraft enforces the single-use review sequence using the
// supplied state seams.
func handleSharedSendDraft(state *sharedSendState, codec *resource.ReferenceCodec) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		target, err := mailTargetFromContext(ctx)
		if err != nil || !target.isShared() {
			return mcp.NewToolResultError("shared send requires a resolved shared mailbox"), nil
		}
		draftID, err := target.draftID("")
		if err != nil || codec == nil || request.GetString("draft_ref", "") == "" {
			return mcp.NewToolResultError("shared send requires a verified draft_ref"), nil
		}
		reviewed, err := state.load(ctx, target, draftID)
		if err != nil {
			return mcp.NewToolResultError(sharedSendReadError(target, err)), nil
		}
		recordSharedSendEvidence(ctx, codec, request.GetString("draft_ref", ""), reviewed)
		auth.RecordAuditConfirmation(ctx, "requested")
		result, err := state.elicit(ctx, sharedSendElicitation(reviewed))
		if err != nil {
			auth.RecordAuditConfirmation(ctx, "unsupported_or_failed")
			return mcp.NewToolResultError("shared send requires fresh human confirmation through MCP elicitation; this client does not support it"), nil
		}
		if result == nil || result.Action != mcp.ElicitationResponseActionAccept || !elicitationConfirmed(result.Content) {
			auth.RecordAuditConfirmation(ctx, "declined_or_cancelled")
			auth.RecordAuditOutcome(ctx, "canceled")
			return mcp.NewToolResultText("Shared draft send cancelled; no message was sent."), nil
		}
		if err := revalidateMailTarget(ctx, target); err != nil {
			auth.RecordAuditConfirmation(ctx, "authorization_changed")
			return mcp.NewToolResultError(err.Error()), nil
		}
		current, err := state.load(ctx, target, draftID)
		if err != nil {
			auth.RecordAuditConfirmation(ctx, "review_refresh_failed")
			return mcp.NewToolResultError(sharedSendReadError(target, err)), nil
		}
		if !reflect.DeepEqual(reviewed, current) {
			auth.RecordAuditConfirmation(ctx, "stale")
			return mcp.NewToolResultError("shared draft changed after confirmation; review and confirm it again"), nil
		}
		if err := revalidateMailTarget(ctx, target); err != nil {
			auth.RecordAuditConfirmation(ctx, "authorization_changed")
			return mcp.NewToolResultError(err.Error()), nil
		}
		auth.RecordAuditConfirmation(ctx, "accepted_send_attempted")
		auth.MarkAuditSendAttempt(ctx)
		if err := state.send(ctx, target, draftID); err != nil {
			status := graph.ExtractHTTPStatus(err)
			if status == 0 || status >= 500 {
				auth.RecordAuditOutcome(ctx, "uncertain")
			} else {
				auth.RecordAuditOutcome(ctx, "denied")
			}
			return mcp.NewToolResultError(sharedSendMutationError(target, err)), nil
		}
		auth.RecordAuditOutcome(ctx, "accepted")
		response := "Shared draft accepted by Microsoft Graph for Exchange processing; this does not confirm delivery. Exchange chooses Send As or Send on Behalf from configured rights. The owner's Sent Items is the documented default, not an exclusive guarantee under tenant policy."
		response += fmt.Sprintf("\nShared Target: %s (%s)", target.target.Alias, logging.MaskEmail(target.target.Owner))
		if line := AccountInfoLine(ctx); line != "" {
			response += "\n" + line
		}
		return mcp.NewToolResultText(response), nil
	}
}

// recordSharedSendEvidence stores only counts and keyed fingerprints so audit
// logs can correlate a review without exposing addresses or message metadata.
func recordSharedSendEvidence(ctx context.Context, codec *resource.ReferenceCodec, reference string, snapshot sharedSendSnapshot) {
	if codec == nil {
		return
	}
	recipients := strings.Join(snapshot.To, "\x00") + "\x01" + strings.Join(snapshot.CC, "\x00") + "\x01" + strings.Join(snapshot.BCC, "\x00")
	attachmentParts := make([]string, 0, len(snapshot.Attachments))
	for _, attachment := range snapshot.Attachments {
		attachmentParts = append(attachmentParts, fmt.Sprintf("%s\x00%s\x00%d\x00%s\x00%t", attachment.ID, attachment.Name, attachment.Size, attachment.ContentType, attachment.Inline))
	}
	auth.RecordAuditSendEvidence(ctx, auth.AuditSendEvidence{
		RecipientCount: len(snapshot.To) + len(snapshot.CC) + len(snapshot.BCC), AttachmentCount: len(snapshot.Attachments),
		ReferenceFingerprint: codec.KeyedFingerprint("reference", reference), ChangeKeyFingerprint: codec.KeyedFingerprint("change_key", snapshot.ChangeKey),
		SubjectFingerprint: codec.KeyedFingerprint("subject", snapshot.Subject), RecipientFingerprint: codec.KeyedFingerprint("recipients", recipients),
		AttachmentFingerprint: codec.KeyedFingerprint("attachments", strings.Join(attachmentParts, "\x01")),
	})
}

// loadSharedSendSnapshot fetches the draft and every attachment page through
// the immutable owner route and rejects incomplete send evidence.
func loadSharedSendSnapshot(ctx context.Context, target mailReadTarget, retryCfg graph.RetryConfig, timeout time.Duration, draftID string) (sharedSendSnapshot, error) {
	timeoutCtx, cancel := graph.WithTimeout(ctx, timeout)
	defer cancel()
	var message models.Messageable
	if err := graph.RetryGraphCall(ctx, retryCfg, func() error {
		var callErr error
		message, callErr = target.root.Messages().ByMessageId(draftID).Get(timeoutCtx, nil)
		return callErr
	}); err != nil {
		return sharedSendSnapshot{}, err
	}
	if message == nil || !graph.SafeBool(message.GetIsDraft()) || graph.SafeStr(message.GetChangeKey()) == "" || message.GetFrom() == nil || message.GetFrom().GetEmailAddress() == nil {
		return sharedSendSnapshot{}, fmt.Errorf("shared send requires an existing draft with canonical From and nonempty changeKey")
	}
	from := normalizeAddress(graph.SafeStr(message.GetFrom().GetEmailAddress().GetAddress()))
	if from == "" || from != normalizeAddress(target.target.Owner) {
		return sharedSendSnapshot{}, fmt.Errorf("shared draft From does not match the configured owner mailbox")
	}
	snapshot := sharedSendSnapshot{
		AccountID: string(target.target.AccountID), ResourceID: string(target.target.ResourceID), Alias: target.target.Alias,
		Owner: target.target.Owner, View: string(target.target.View), From: from,
		ChangeKey: graph.SafeStr(message.GetChangeKey()), Subject: graph.SafeStr(message.GetSubject()),
		To:  normalizeRecipients(recipientAddresses(message.GetToRecipients())),
		CC:  normalizeRecipients(recipientAddresses(message.GetCcRecipients())),
		BCC: normalizeRecipients(recipientAddresses(message.GetBccRecipients())),
	}
	if info, ok := auth.AccountInfoFromContext(ctx); ok {
		snapshot.DelegateLabel = info.Label
		snapshot.DelegateUPN = info.Email
	}
	if len(snapshot.To)+len(snapshot.CC)+len(snapshot.BCC) == 0 {
		return sharedSendSnapshot{}, fmt.Errorf("shared send requires at least one recipient")
	}
	attachments, err := loadAllSharedSendAttachments(timeoutCtx, target, retryCfg, draftID)
	if err != nil {
		return sharedSendSnapshot{}, err
	}
	snapshot.Attachments = attachments
	return snapshot, nil
}

// loadAllSharedSendAttachments follows only owner-route next links and returns
// a stable complete attachment set without downloading content bytes.
func loadAllSharedSendAttachments(ctx context.Context, target mailReadTarget, retryCfg graph.RetryConfig, draftID string) ([]sharedSendAttachment, error) {
	builder := target.root.Messages().ByMessageId(draftID).Attachments()
	var response models.AttachmentCollectionResponseable
	if err := graph.RetryGraphCall(ctx, retryCfg, func() error {
		var callErr error
		response, callErr = builder.Get(ctx, nil)
		return callErr
	}); err != nil {
		return nil, err
	}
	attachments := make([]sharedSendAttachment, 0)
	for response != nil {
		for _, attachment := range response.GetValue() {
			if attachment == nil || graph.SafeStr(attachment.GetId()) == "" || graph.SafeStr(attachment.GetName()) == "" ||
				graph.SafeStr(attachment.GetContentType()) == "" || attachment.GetSize() == nil || attachment.GetIsInline() == nil {
				return nil, fmt.Errorf("shared send attachment evidence is incomplete")
			}
			attachments = append(attachments, sharedSendAttachment{
				ID: graph.SafeStr(attachment.GetId()), Name: graph.SafeStr(attachment.GetName()),
				ContentType: graph.SafeStr(attachment.GetContentType()), Size: graph.SafeInt32(attachment.GetSize()),
				Inline: graph.SafeBool(attachment.GetIsInline()),
			})
		}
		next := graph.SafeStr(response.GetOdataNextLink())
		if next == "" {
			break
		}
		if err := validateSharedAttachmentNextLink(next, target.target.Owner, draftID); err != nil {
			return nil, err
		}
		if err := graph.RetryGraphCall(ctx, retryCfg, func() error {
			var callErr error
			response, callErr = builder.WithUrl(next).Get(ctx, nil)
			return callErr
		}); err != nil {
			return nil, err
		}
	}
	sort.Slice(attachments, func(i, j int) bool {
		return fmt.Sprintf("%s\x00%s\x00%d\x00%s\x00%t", attachments[i].ID, attachments[i].Name, attachments[i].Size, attachments[i].ContentType, attachments[i].Inline) <
			fmt.Sprintf("%s\x00%s\x00%d\x00%s\x00%t", attachments[j].ID, attachments[j].Name, attachments[j].Size, attachments[j].ContentType, attachments[j].Inline)
	})
	return attachments, nil
}

// validateSharedAttachmentNextLink rejects pagination that leaves the exact
// configured owner and draft attachment collection route.
func validateSharedAttachmentNextLink(raw, owner, draftID string) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || !strings.EqualFold(parsed.Hostname(), "graph.microsoft.com") ||
		(parsed.Port() != "" && parsed.Port() != "443") || parsed.User != nil {
		return fmt.Errorf("shared send attachment pagination returned an invalid next link")
	}
	want := "/v1.0/users/" + owner + "/messages/" + draftID + "/attachments"
	if parsed.Path != want {
		return fmt.Errorf("shared send attachment pagination changed the immutable owner route")
	}
	return nil
}

// normalizeRecipients returns a sorted lower-case multiset while preserving
// duplicates, because duplicate recipients are part of the reviewed evidence.
func normalizeRecipients(values []string) []string {
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		if address := normalizeAddress(value); address != "" {
			normalized = append(normalized, address)
		}
	}
	sort.Strings(normalized)
	return normalized
}

// normalizeAddress canonicalizes one mailbox address for exact comparison.
func normalizeAddress(value string) string { return strings.ToLower(strings.TrimSpace(value)) }

// sharedSendElicitation presents the complete masked review without copying
// the body and states the non-predictive Exchange sender semantics.
func sharedSendElicitation(snapshot sharedSendSnapshot) mcp.ElicitationRequest {
	delegate := "Account ID: " + snapshot.AccountID
	if snapshot.DelegateLabel != "" || snapshot.DelegateUPN != "" {
		delegate = fmt.Sprintf("Delegate: %s (%s)", snapshot.DelegateLabel, logging.MaskEmail(snapshot.DelegateUPN))
	}
	attachments := make([]string, 0, len(snapshot.Attachments))
	for _, attachment := range snapshot.Attachments {
		attachments = append(attachments, fmt.Sprintf("%s (%d bytes, %s, inline=%t)", attachment.Name, attachment.Size, attachment.ContentType, attachment.Inline))
	}
	message := fmt.Sprintf("Send this shared-mailbox draft?\n%s\nShared mailbox: %s (%s)\nCanonical From: %s\nSubject: %s\nTo: %s\nCc: %s\nBcc: %s\nAttachments: %s\n\nThe message body is not included; review it in Outlook before confirming. Exchange chooses Send As or Send on Behalf from configured rights. The owner's Sent Items is the documented default, not an exclusive guarantee.",
		delegate, snapshot.Alias, logging.MaskEmail(snapshot.Owner), logging.MaskEmail(snapshot.From), snapshot.Subject,
		joinedOrNone(maskAddresses(snapshot.To)), joinedOrNone(maskAddresses(snapshot.CC)), joinedOrNone(maskAddresses(snapshot.BCC)), joinedOrNone(attachments))
	return mcp.ElicitationRequest{Params: mcp.ElicitationParams{Message: message, RequestedSchema: map[string]any{
		"type": "object", "properties": map[string]any{"confirmed": map[string]any{"type": "boolean", "description": "Confirm sending this exact unchanged shared draft."}}, "required": []string{"confirmed"},
	}}}
}

// maskAddresses masks presentation-only recipient values.
func maskAddresses(values []string) []string {
	masked := make([]string, len(values))
	for index, value := range values {
		masked[index] = logging.MaskEmail(value)
	}
	return masked
}

// sharedSendReadError explains the distinct OAuth and Exchange authorization
// layers without claiming an unobserved exact cause.
func sharedSendReadError(target mailReadTarget, err error) string {
	return target.graphError(err) + ". Check Mail.ReadWrite.Shared, Full Access or folder access, and tenant policy; OAuth consent does not grant Exchange delegation."
}

// sharedSendMutationError distinguishes explicit authorization rejection from
// an ambiguous one-attempt send result.
func sharedSendMutationError(target mailReadTarget, err error) string {
	detail := strings.ToLower(err.Error())
	if strings.Contains(detail, "errorsendasdenied") || strings.Contains(detail, "send as denied") {
		return "Exchange rejected the shared sender. Check Send As or Send on Behalf rights; OAuth Mail.Send.Shared consent does not grant either Exchange right. Detail: " + target.graphError(err)
	}
	status := graph.ExtractHTTPStatus(err)
	if status == 0 || status >= 500 {
		return "shared send outcome is uncertain; do not retry. Inspect the owner's Drafts and Sent Items before a new human-reviewed attempt. Detail: " + target.graphError(err)
	}
	return target.graphError(err) + ". Check Mail.Send.Shared plus Exchange Send As or Send on Behalf rights without assuming which right is missing."
}
