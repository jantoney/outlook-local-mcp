package tools

import (
	"fmt"

	"github.com/desek/outlook-local-mcp/internal/resource"
)

// addSharedMailReferences adds direct text/summary provenance or a raw
// position-aligned sidecar without changing Graph-derived data maps.
func addSharedMailReferences(items []map[string]any, target mailReadTarget, codec *resource.ReferenceCodec, kind resource.ItemKind, raw bool) (any, error) {
	return addMailReferences(items, target, codec, kind, raw, target.isShared())
}

// addMailReferences signs target-bound item references when enabled. It lets
// existing own reads opt in without changing their default output shapes.
func addMailReferences(items []map[string]any, target mailReadTarget, codec *resource.ReferenceCodec, kind resource.ItemKind, raw, enabled bool) (any, error) {
	if !enabled {
		return items, nil
	}
	if codec == nil {
		return nil, fmt.Errorf("shared resource reference signing is unavailable")
	}
	provenance := make([]map[string]any, 0, len(items))
	for index, item := range items {
		reference, err := signMailItem(codec, target.target, kind, eventString(item, "id"))
		if err != nil {
			return nil, err
		}
		if raw {
			provenance = append(provenance, map[string]any{"item_index": index, "resource_ref": reference})
		} else {
			item["resource_ref"] = reference
		}
	}
	if raw {
		return map[string]any{"data": items, "provenance": provenance}, nil
	}
	return items, nil
}

// addSharedMailReference adds provenance for one shared message result.
func addSharedMailReference(item map[string]any, target mailReadTarget, codec *resource.ReferenceCodec, raw bool) (any, error) {
	wrapped, err := addSharedMailReferences([]map[string]any{item}, target, codec, resource.ItemKindMessage, raw)
	if err != nil || !target.isShared() {
		return item, err
	}
	if raw {
		container := wrapped.(map[string]any)
		refs := container["provenance"].([]map[string]any)
		return map[string]any{"data": item, "provenance": map[string]any{"resource_ref": refs[0]["resource_ref"]}}, nil
	}
	return item, nil
}

// signMailItem creates one target-bound folder or message reference.
func signMailItem(codec *resource.ReferenceCodec, target resource.Target, kind resource.ItemKind, graphID string) (string, error) {
	if graphID == "" {
		return "", fmt.Errorf("shared mailbox %s is missing its Graph ID", kind)
	}
	return codec.Sign(resource.ReferenceClaims{
		AccountID: target.AccountID, ResourceID: target.ResourceID,
		ResourceKind: target.Kind, MailboxView: target.View, ItemKind: kind,
		GraphIDChain: []resource.GraphID{{Kind: kind, ID: graphID}},
	})
}

// appendSharedDraftReference adds a renewed target-bound draft reference to a
// successful shared-mail write confirmation. Own-mail confirmations are
// returned unchanged.
func appendSharedDraftReference(response string, target mailReadTarget, codec *resource.ReferenceCodec, draftID string) (string, error) {
	if !target.isShared() {
		return response, nil
	}
	if codec == nil {
		return "", fmt.Errorf("shared resource reference signing is unavailable")
	}
	reference, err := signMailItem(codec, target.target, resource.ItemKindDraft, draftID)
	if err != nil {
		return "", err
	}
	return response + "\nDraft Ref: " + reference, nil
}

// addSharedAttachmentReferences signs attachment metadata with its verified
// parent message chain and preserves raw maps through a sidecar.
func addSharedAttachmentReferences(items []map[string]any, target mailReadTarget, codec *resource.ReferenceCodec, messageID string, raw bool) (any, error) {
	if !target.isShared() {
		return items, nil
	}
	if codec == nil {
		return nil, fmt.Errorf("shared resource reference signing is unavailable")
	}
	provenance := make([]map[string]any, 0, len(items))
	for index, item := range items {
		attachmentID := eventString(item, "id")
		if messageID == "" || attachmentID == "" {
			return nil, fmt.Errorf("shared attachment provenance requires parent message and attachment IDs")
		}
		reference, err := codec.Sign(resource.ReferenceClaims{
			AccountID: target.target.AccountID, ResourceID: target.target.ResourceID,
			ResourceKind: target.target.Kind, MailboxView: target.target.View, ItemKind: resource.ItemKindAttachment,
			GraphIDChain: []resource.GraphID{
				{Kind: resource.ItemKindMessage, ID: messageID},
				{Kind: resource.ItemKindAttachment, ID: attachmentID},
			},
		})
		if err != nil {
			return nil, err
		}
		if raw {
			provenance = append(provenance, map[string]any{"item_index": index, "attachment_ref": reference})
		} else {
			item["attachment_ref"] = reference
		}
	}
	if raw {
		return map[string]any{"data": items, "provenance": provenance}, nil
	}
	return items, nil
}

// addSharedAttachmentReference renews provenance for one downloaded attachment.
func addSharedAttachmentReference(item map[string]any, target mailReadTarget, codec *resource.ReferenceCodec, messageID string, raw bool) (any, error) {
	wrapped, err := addSharedAttachmentReferences([]map[string]any{item}, target, codec, messageID, raw)
	if err != nil || !target.isShared() {
		return item, err
	}
	if raw {
		container := wrapped.(map[string]any)
		refs := container["provenance"].([]map[string]any)
		return map[string]any{"data": item, "provenance": map[string]any{"attachment_ref": refs[0]["attachment_ref"]}}, nil
	}
	return item, nil
}
