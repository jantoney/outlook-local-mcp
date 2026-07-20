# Verify the shared mail and send Graph contract

Labels: `wayfinder:research`
Status: closed
Assignee: /root/research_shared_mail

## Question

For every mail operation included by CR-0067, which Microsoft Graph v1.0 owner
routes, delegated `.Shared` scopes, Exchange delegation rights, draft rules,
attachment behavior, and Send As or Send on Behalf semantics are authoritative?

## Evidence to verify

- Shared folder and mailbox access through `/users/{owner}/...`.
- Work/school-only constraints for shared mail scopes.
- Draft creation, reply, forward, update, delete, and attachment support.
- `Mail.Send.Shared` interaction with `Mail.ReadWrite.Shared` and Exchange rights.
- Sender, From, Sent Items, stale draft, retry, and confirmation semantics.

## Resolution

Verified against Microsoft Graph v1.0 and Exchange Online documentation on
2026-07-19. The authoritative contract for CR-0067 is as follows.

### Target and permission boundary

1. Direct access to another user's/shared mailbox uses that mailbox owner's
   user ID or UPN in `/users/{owner}/...`; it does not use `/me`. Microsoft
   explicitly documents owner routes for folders, messages, drafts, reply and
   forward drafts, attachments, and draft send. IDs are mailbox-route scoped in
   practice and MUST remain associated with the configured owner target.
2. `Mail.Read.Shared` authorizes reading mail the signed-in user can access,
   including shared mail. `Mail.ReadWrite.Shared` authorizes creating, reading,
   updating, and deleting accessible own/shared mail, but explicitly does not
   authorize send. Both are delegated-only and valid only for work or school
   accounts.
3. OAuth scope is only the application authorization layer. The owner must also
   grant Exchange/Outlook resource access. A folder share can grant read,
   create, modify, or delete access to individual folders; delegation or Full
   Access can cover the mailbox. Graph returns an error when the signed-in user
   has neither sharing nor delegation for the requested owner resource.
4. The operation-reference pages generally list the non-shared delegated scope
   (`Mail.Read` or `Mail.ReadWrite`) even when they show `/users/{owner}` routes.
   For a signed-in delegate accessing shared data, Microsoft's shared-folder
   guidance and permission reference are the controlling scope descriptions:
   use `Mail.Read.Shared` for reads and `Mail.ReadWrite.Shared` for mutations.
5. `Mail.Send.Shared` is delegated-only, work/school-only, and separate from
   `Mail.ReadWrite.Shared`. Microsoft says it can send and save a sent copy even
   without either read/write scope. CR-0067 nevertheless correctly needs
   `Mail.ReadWrite.Shared` as well because its workflow creates/reads/updates an
   existing draft before sending it.

### Authoritative v1.0 owner routes for the CR mail matrix

Use the equivalent folder-qualified form when the handler already knows a
folder; otherwise the mailbox-wide form is sufficient.

| Capability | Owner route |
|---|---|
| List folders | `GET /users/{owner}/mailFolders` and child-folder traversal |
| List/search/conversation grouping | `GET /users/{owner}/messages` or `GET /users/{owner}/mailFolders/{folder-id}/messages`, with OData query options; conversations are derived from message `conversationId` |
| Get a message | `GET /users/{owner}/messages/{message-id}` or the folder-qualified form |
| Create a new draft | `POST /users/{owner}/messages` or `POST /users/{owner}/mailFolders/{folder-id}/messages` |
| Update a draft | `PATCH /users/{owner}/messages/{message-id}` or the folder-qualified form |
| Delete a draft/message | `DELETE /users/{owner}/messages/{message-id}` or the folder-qualified form |
| Create reply draft | `POST /users/{owner}/messages/{message-id}/createReply` or the folder-qualified form |
| Create forward draft | `POST /users/{owner}/messages/{message-id}/createForward` or the folder-qualified form |
| Read/add/delete attachment | `/users/{owner}/messages/{message-id}/attachments[/{attachment-id}]`, with the documented HTTP verb; folder-qualified forms are also available |
| Start large attachment upload | `POST /users/{owner}/messages/{message-id}/attachments/createUploadSession` |
| Send existing draft | `POST /users/{owner}/messages/{message-id}/send` with an empty body |

Creating a message produces a draft in Drafts by default and returns `201` plus
the message. Reply and forward draft actions also return `201` plus the draft.
Draft-only fields such as subject, body, and recipients are updatable only while
`isDraft=true`. Delete returns `204`. Sending accepts a new, reply, reply-all,
or forward draft and returns `202 Accepted` with no body.

### Drafts, attachments, and owner-mailbox access

- A draft created under `/users/{owner}` resides in the owner's mailbox. Folder
  permission/delegation must cover the relevant source folder and Drafts, or
  the delegate needs mailbox-wide Full Access. Merely having a `.Shared` OAuth
  scope does not create that access.
- JSON draft creation can include attachments. For an existing message, direct
  attachment POST supports file, item, and reference attachments and is
  documented for attachments under 3 MB. For files from 3 MB through 150 MB,
  create an upload session and upload sequential byte ranges (up to 4 MB per
  request) to the returned opaque, expiring URL. The upload URL already contains
  authorization and MUST NOT be logged, modified, or sent with an Authorization
  header.
- Tenant message-size policy can be lower than the API's upload-session maximum
  (Microsoft documents a default Exchange Online message size limit of 35 MB).
  The upload-session documentation also links an active known issue for large
  files in shared or delegated mailboxes. Shared large-attachment tests must be
  treated as required compatibility tests, not assumed from `/me` behavior.
- Attachment and draft mutations require `Mail.ReadWrite.Shared` plus actual
  Exchange write access. `Mail.Send.Shared` alone is insufficient to prepare or
  inspect the draft even though it is independently sufficient for the send
  authorization layer.

### Send As, Send on Behalf, From, Sender, and Sent Items

1. For a user-token send from another mailbox, Microsoft requires
   `Mail.Send.Shared` and at least one Exchange right on the From mailbox:
   **Send As** or **Send on Behalf**. Full Access by itself permits opening and
   modifying mailbox content but explicitly does not permit sending.
2. CR-0067 chooses `POST /users/{owner}/messages/{draft-id}/send`, where the
   route user is also the From user. Microsoft explicitly requires **Full
   Access in addition to Send As or Send on Behalf** for this owner-routed form.
   Therefore the implementation/setup documentation must not describe only a
   shared-write right plus a send right; owner-route shared draft send needs all
   three layers: `Mail.ReadWrite.Shared` for draft management,
   `Mail.Send.Shared` for Graph send, and Exchange Full Access plus Send As or
   Send on Behalf.
3. The draft's `from` address must be the configured owner/From mailbox.
   Microsoft's guidance says to set `from` and not set `sender`; Graph assigns
   `sender` based on the signed-in principal and Exchange rights. Before
   confirmation and again immediately before send, reject a draft whose From
   address does not equal the configured owner.
4. With Send on Behalf, `sender` is the signed-in delegate and `from` is the
   owner; recipients see “delegate on behalf of owner.” With Send As, there is
   no delegate indication and `sender` equals `from` (the owner). The MCP must
   describe these as Exchange-selected semantics and must not promise one when
   it has not independently established which Exchange right is configured;
   Graph cannot enumerate the mailboxes for which the user has these rights.
5. With `/users/{owner}` where owner is the From user, the default sent copy is
   in the owner's Sent Items. Tenant settings can additionally control delegate
   copies. This differs from sending via `/me`, whose default is the signed-in
   sender's Sent Items. The existing-draft send action has no
   `saveToSentItems` request body, so CR-0067 should report the documented
   owner-route default without claiming it can override tenant policy.
6. Lack of the Exchange send right is documented as `403 Forbidden` with
   `ErrorSendAsDenied`. Errors should distinguish this from missing OAuth scope
   or missing Full Access.

### Confirmation, version binding, acceptance, and retry

- Human confirmation is an MCP safety control, not a Graph feature. Graph's
  draft-send action accepts only the message ID, has an empty body, and provides
  no confirmation token or user-review primitive. The elicitation must therefore
  bind to the fields fetched by the application (account, owner/alias, From
  semantics, recipients, subject, attachments, and draft version).
- The message `changeKey` is documented as the version of the message. A robust
  pre-send check re-fetches the owner-routed message immediately after accepted
  elicitation and verifies `isDraft=true`, the same `changeKey`, expected From,
  recipients, subject, and attachment metadata before issuing send.
- Microsoft does **not** document an `If-Match`/changeKey precondition on the
  v1.0 draft-send action. Consequently, a pre-send GET followed by POST has a
  small time-of-check/time-of-use race. CR-0067 can fail closed on every stale
  version it observes, but it must not claim an atomic “send only this exact
  version” guarantee from Graph. If an atomic guarantee remains an acceptance
  requirement, that requirement is not satisfied by the documented v1.0 API.
- A successful send is only `202 Accepted` with no response body. The response
  must say accepted for Exchange processing, not delivered and not necessarily
  already present in Sent Items.
- The send POST has no documented idempotency key or replay contract. Network
  loss or a 5xx after submission is ambiguous. Do not retry an ambiguous send.
  This includes disabling/bypassing default SDK retry middleware for this call:
  Kiota documents that its default retry handler automatically retries `429`
  and `503`, which is unsafe for an unconditionally replayed send POST. A clear
  pre-submission `429` may be theoretically retryable, but the fail-closed rule
  is simpler and consistent with CR-0067: return an uncertain/not-sent status
  and require the user to inspect Drafts/Sent Items before a new attempt.

## Sources

Primary Microsoft sources only:

- Microsoft Graph, shared/delegated mail folders:
  https://learn.microsoft.com/en-us/graph/outlook-share-messages-folders
- Microsoft Graph permissions reference (`Mail.Read.Shared`,
  `Mail.ReadWrite.Shared`, `Mail.Send.Shared`):
  https://learn.microsoft.com/en-us/graph/permissions-reference
- Microsoft Graph, send mail from another user:
  https://learn.microsoft.com/en-us/graph/outlook-send-mail-from-other-user
- Exchange Online recipient permissions (Full Access, Send As, Send on Behalf):
  https://learn.microsoft.com/en-us/exchange/recipients-in-exchange-online/manage-permissions-for-recipients
- Microsoft Graph v1.0 create/update/delete/send message:
  https://learn.microsoft.com/en-us/graph/api/user-post-messages?view=graph-rest-1.0
  https://learn.microsoft.com/en-us/graph/api/message-update?view=graph-rest-1.0
  https://learn.microsoft.com/en-us/graph/api/message-delete?view=graph-rest-1.0
  https://learn.microsoft.com/en-us/graph/api/message-send?view=graph-rest-1.0
- Microsoft Graph v1.0 reply/forward draft actions:
  https://learn.microsoft.com/en-us/graph/api/message-createreply?view=graph-rest-1.0
  https://learn.microsoft.com/en-us/graph/api/message-createforward?view=graph-rest-1.0
- Microsoft Graph v1.0 attachments:
  https://learn.microsoft.com/en-us/graph/api/message-post-attachments?view=graph-rest-1.0
  https://learn.microsoft.com/en-us/graph/api/attachment-createuploadsession?view=graph-rest-1.0
  https://learn.microsoft.com/en-us/graph/api/attachment-delete?view=graph-rest-1.0
- Microsoft Graph v1.0 message resource (`changeKey`, `conversationId`,
  `sender`, `from`, `isDraft`):
  https://learn.microsoft.com/en-us/graph/api/resources/message?view=graph-rest-1.0
- Microsoft Kiota middleware (default retry behavior):
  https://learn.microsoft.com/en-us/openapi/kiota/middleware
