# Verify mail move, delete, and archive Graph contract

Labels: `wayfinder:research`
Status: closed
Assignee: /root/research_mail_mutations

## Question

Which Microsoft Graph message operations and endpoint shapes distinguish moving
a message to an ordinary folder, moving it to Archive, deleting it, moving it
to Deleted Items, and restoring it, and can the MCP reliably gate those actions
independently before constructing a Graph request?

## Evidence to verify

- Official Microsoft Graph message move and delete API documentation.
- Whether Archive and Deleted Items are well-known folders or distinct message
  action endpoints.
- Soft-delete versus permanent-delete behavior available through Graph v1.0.
- Relevant request builders in the installed Microsoft Graph Go SDK v1.96.0.
- Any identifier changes or response semantics caused by a move.

## Findings

Microsoft Graph v1.0 exposes three relevant message mutation shapes, not one
undifferentiated `manage` operation:

| User intent | Graph v1.0 request | Result | Independently gateable? |
| --- | --- | --- | --- |
| Move to an ordinary folder | `POST /users/{user}/messages/{message}/move` with `{"destinationId":"<folder-id>"}` | `201 Created` plus the message in its new folder | Yes: classify the validated destination before building the request. |
| Archive | The same `/move` action with `destinationId: "archive"` | Moves to Outlook's One-Click Archive folder; this is not the Exchange Online Archive Mailbox | Yes: `archive` is a well-known destination, so it can have a separate policy gate even though the wire endpoint is shared. |
| Move to Deleted Items (trash) | The same `/move` action with `destinationId: "deleteditems"` | Moves to the well-known Deleted Items folder and returns the moved message | Yes: reserve `deleteditems` as a delete/trash-class destination rather than allowing it through an ordinary-move gate. |
| Delete | `DELETE /users/{user}/messages/{message}` (folder-qualified form is also available) | `204 No Content` | Yes: this is a distinct request method/builder. Deleting an item from Deleted Items places it in `recoverableitemsdeletions`; Microsoft warns that deleting from that recoverable folder might not be allowed. |
| Restore from Deleted Items | The `/move` action, with an allowed non-deletion destination | `201 Created` plus the restored/moved message | Yes: Graph v1.0 has no message-specific `restore` action; restoration is a policy-qualified move. |
| Permanent delete | `POST /users/{user}/messages/{message}/permanentDelete` (folder-qualified form is also available) | `204 No Content`; places the message in the hidden Purges area until retention disposal, unless a mailbox hold applies | Yes: this is a distinct action and should have its own highest-risk gate. |

The move API accepts either a folder ID or a well-known folder name. Therefore
the MCP can gate all of these intents before request construction: normalize and
validate the destination, reject deletion-class well-known destinations from the
ordinary-move capability, and dispatch only after the corresponding capability
has passed. A raw caller-supplied folder ID needs resolution to a folder (or an
explicit policy decision) before it can safely be classified; string comparison
alone only protects the well-known-name forms.

The installed `github.com/microsoftgraph/msgraph-sdk-go` v1.96.0 generated
surface matches the REST contract: message request builders expose `Move().Post`
with a typed `MovePostRequestBody` carrying `destinationId`, `Delete`, and
`PermanentDelete().Post`. It provides no archive-specific or message-restore
request builder. Archive, trash, and restore therefore remain application-level
classifications around `Move`, while delete and permanent delete are separate
SDK calls. All three documented mutations use `Mail.ReadWrite`, so OAuth scopes
cannot provide this finer separation.

This supports restoring a message that remains in Deleted Items by moving it.
The v1.0 documentation identifies `recoverableitemsdeletions` and the Outlook
recovery experience, but does not document a message `restore` action from that
hidden folder; do not promise that recovery path without separate live
verification.

## Identifier semantics

The move operation creates a new copy in the destination and removes the
original. Its `201` response contains the destination message, including its
post-move ID; callers must use that returned resource rather than assume the
input ID remains valid. Default Outlook REST IDs can change when an item moves.
If every relevant request opts in with `Prefer: IdType="ImmutableId"`, the ID
remains stable while the item stays in the same mailbox. Moving to the separate
Archive Mailbox is an explicit exception; the well-known `archive` folder is not
that mailbox.

## Sources

- Microsoft Graph v1.0, [message: move](https://learn.microsoft.com/en-us/graph/api/message-move?view=graph-rest-1.0) — endpoint forms, `destinationId`, copy/remove semantics, and `201` response.
- Microsoft Graph v1.0, [Delete message](https://learn.microsoft.com/en-us/graph/api/message-delete?view=graph-rest-1.0) — `DELETE` endpoint forms, `204` response, and recoverable-items limitation.
- Microsoft Graph v1.0, [message: permanentDelete](https://learn.microsoft.com/en-us/graph/api/message-permanentdelete?view=graph-rest-1.0) — distinct action, Purges/retention semantics, endpoint forms, and `204` response.
- Microsoft Graph v1.0, [mailFolder resource](https://learn.microsoft.com/en-us/graph/api/resources/mailfolder?view=graph-rest-1.0) — `archive`, `deleteditems`, and `recoverableitemsdeletions` well-known folder meanings.
- Microsoft Graph, [Obtain immutable identifiers for Outlook resources](https://learn.microsoft.com/en-us/graph/outlook-immutable-id) — default move-sensitive IDs, opt-in header, same-mailbox stability, and Archive Mailbox exception.
- Installed generated source: `C:/Users/jayan/go/pkg/mod/github.com/microsoftgraph/msgraph-sdk-go@v1.96.0/users` — request-builder and typed-body verification against the pinned dependency.
