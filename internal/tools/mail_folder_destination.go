package tools

import (
	"context"
	"fmt"

	"github.com/desek/outlook-local-mcp/internal/graph"
	"github.com/desek/outlook-local-mcp/internal/resource"
)

var reservedMailFolderNames = []string{
	"archive", "clutter", "conflicts", "conversationhistory", "deleteditems",
	"drafts", "inbox", "junkemail", "localfailures", "msgfolderroot", "outbox",
	"recoverableitemsdeletions", "recoverableitemspurges", "recoverableitemsroot",
	"recoverableitemsversions", "scheduled", "searchfolders", "sentitems",
	"serverfailures", "syncissues",
}

// resolveReservedMailFolderIDs resolves the mailbox's well-known system folder
// IDs before move references are signed. Missing optional folders are ignored;
// every other Graph error fails closed.
func resolveReservedMailFolderIDs(ctx context.Context, target mailReadTarget, retryCfg graph.RetryConfig) (map[string]bool, error) {
	reserved := make(map[string]bool)
	for _, name := range reservedMailFolderNames {
		var id string
		err := graph.RetryGraphCall(ctx, retryCfg, func() error {
			folder, callErr := target.root.MailFolders().ByMailFolderId(name).Get(ctx, nil)
			if folder != nil {
				id = graph.SafeStr(folder.GetId())
			}
			return callErr
		})
		if graph.ExtractHTTPStatus(err) == 404 {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("resolve reserved mail folder %q: %w", name, err)
		}
		if id != "" {
			reserved[id] = true
		}
	}
	return reserved, nil
}

// addMoveFolderReferences classifies and signs folder results for explicit
// use as move destinations while preserving raw Graph maps behind a sidecar.
func addMoveFolderReferences(items []map[string]any, target mailReadTarget, codec *resource.ReferenceCodec, reserved map[string]bool, raw bool) (any, error) {
	if codec == nil {
		return nil, fmt.Errorf("mail folder reference signing is unavailable")
	}
	provenance := make([]map[string]any, 0, len(items))
	for index, item := range items {
		id := eventString(item, "id")
		class := resource.MailFolderClassOrdinary
		if reserved[id] {
			class = resource.MailFolderClassReserved
		}
		reference, err := codec.Sign(resource.ReferenceClaims{
			AccountID: target.target.AccountID, ResourceID: target.target.ResourceID,
			ResourceKind: target.target.Kind, MailboxView: target.target.View,
			ItemKind: resource.ItemKindMailFolder, MailFolderClass: class,
			GraphIDChain: []resource.GraphID{{Kind: resource.ItemKindMailFolder, ID: id}},
		})
		if err != nil {
			return nil, err
		}
		if raw {
			provenance = append(provenance, map[string]any{"item_index": index, "resource_ref": reference, "destination_class": class})
		} else {
			item["resource_ref"] = reference
			item["destination_class"] = string(class)
		}
	}
	if raw {
		return map[string]any{"data": items, "provenance": provenance}, nil
	}
	return items, nil
}
