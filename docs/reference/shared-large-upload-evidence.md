# Shared large-attachment upload-session evidence

Status: **unsupported — required live evidence unavailable**  
Decision date: 2026-07-21  
Applies to: organizational shared mailboxes on owner-view routes

## Gate result

The compatibility gate could not be executed because this implementation environment did not provide both a representative organizational shared mailbox and an authorized disposable attachment of at least 3 MiB. No upload session was created and no attachment content, upload URL, token, or mailbox identity was persisted.

| Evidence requirement | Result |
|---|---|
| Owner route | Not exercised; the expected route remains `/users/{owner}/messages/{draft-id}/attachments/createUploadSession` |
| Session creation | Not exercised |
| Sequential range upload | Not exercised |
| Completion response and attachment identity | Not observed |
| Draft verification and cleanup | Not required because no mutation started |
| Observed compatibility | Missing evidence; no positive compatibility claim is authorized |

## Support decision

Large shared-mail attachment upload sessions are unsupported. The public shared attachment contract remains direct upload of allowlisted files smaller than 3 MiB. Files at or above 3 MiB are rejected before session creation, and own-mail resumable upload behavior is unchanged.

No implementation ticket is authorized by this gate. A future positive decision requires a new live run that records the exact owner route, session and range stages, completion behavior, draft verification, and cleanup without retaining sensitive content. Positive evidence must then produce a separate implementation ticket before the public surface changes.
