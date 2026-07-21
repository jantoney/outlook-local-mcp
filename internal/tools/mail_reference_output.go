package tools

import (
	"fmt"

	"github.com/desek/outlook-local-mcp/internal/resource"
)

// addSharedMailReferences adds direct text/summary provenance or a raw
// position-aligned sidecar without changing Graph-derived data maps.
func addSharedMailReferences(items []map[string]any, target mailReadTarget, codec *resource.ReferenceCodec, kind resource.ItemKind, raw bool) (any, error) {
	if !target.isShared() {
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
