# Choose the Graph attachment upload contract

Labels: `wayfinder:research`
Status: closed

## Question

Should `mail.add_attachment` support only Graph's direct small-file attachment POST, or both direct POST and resumable upload sessions, and what size thresholds, retry semantics, confirmation output, and failure behavior should the Change Request require?

## Evidence to verify

- Direct attachment POST supports files under 3 MB.
- Upload sessions support files from 3 MB through 150 MB.
- Upload-session byte ranges must be sequential and chunk sizes must follow Graph constraints.
- The verb must operate only on existing draft messages and require `Mail.ReadWrite`.

## Resolution

Add one write verb, `mail.add_attachment`, that accepts exactly one local file per call and attaches it only to an existing draft after a narrow `$select=id,isDraft` verification.

The handler selects the Graph path by raw file size:

- files smaller than 3 MiB (3,145,728 bytes): direct `POST /me/messages/{id}/attachments` with a `fileAttachment`;
- files from 3 MiB through 150 MiB (157,286,400 bytes): create an attachment upload session and upload sequential 3 MiB byte ranges; and
- files above 150 MiB: reject before upload.

The 150 MiB value is the Graph API ceiling, not a delivery guarantee. Exchange or tenant message-size limits can be lower.

For resumable upload, each PUT uses the opaque pre-authenticated `uploadUrl`, `Content-Type: application/octet-stream`, exact `Content-Length`, and `Content-Range`. It must not add an Authorization header or log the upload URL. The handler follows `nextExpectedRanges`, observes session expiry, and verifies final attachment metadata before reporting success. Intermediate responses are resumable; the final response yields the attachment identity.

The pinned SDK exposes both the direct attachment builder and upload-session creation. Its core `LargeFileUploadTask` is not suitable unchanged because it uses background contexts, hard-coded per-slice timeouts, and retry behavior that does not align with MCP cancellation. Implement a small local uploader around the opaque URL so every phase observes caller cancellation, the configured per-call timeout, and an overall operation deadline.

Direct POST is non-idempotent. Ambiguous transport failures must be reconciled against attachment metadata instead of blindly repeated, to avoid duplicates. Range PUTs are retryable and resumable after refreshing `nextExpectedRanges`. On terminal failure, cancel the known session best-effort and distinguish a confirmed failure from an uncertain outcome.

The verb requires delegated `Mail.ReadWrite`, is write/non-destructive/non-idempotent/open-world, and returns a text confirmation including name, byte size, content type, draft ID, attachment ID, account, and `direct` or `resumable` upload mode.

## Sources

- <https://learn.microsoft.com/en-us/graph/api/message-post-attachments?view=graph-rest-1.0>
- <https://learn.microsoft.com/en-us/graph/api/attachment-createuploadsession?view=graph-rest-1.0>
- <https://learn.microsoft.com/en-us/graph/outlook-large-attachments>
- <https://learn.microsoft.com/en-us/graph/api/message-get?view=graph-rest-1.0>
- <https://learn.microsoft.com/en-us/graph/api/message-update?view=graph-rest-1.0>
- <https://pkg.go.dev/github.com/microsoftgraph/msgraph-sdk-go-core/fileuploader>
