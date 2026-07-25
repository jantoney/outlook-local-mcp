---
name: onedrive-files-and-photos
description: Add gated OneDrive file and photo metadata, photo annotations, personal albums, previews, and recycle-bin deletion.
id: "CR-0073"
status: "proposed"
date: 2026-07-26
requestor: project maintainer
stakeholders: project maintainer, MCP file users, security reviewers
priority: "high"
target-version: next minor release
source-branch: codex/shared-outlook-resources
source-commit: b1dc7a9
---

# OneDrive Files, Photo Metadata, Annotations, Albums, and Deletion

## Change Summary

Add a fifth aggregate MCP domain tool named `drive`. It will reuse the
server's existing account authentication and account-selection model while
letting each account independently expose only the OneDrive actions its owner
has enabled.

The proposed domain covers:

1. file and folder discovery;
2. a complete, paginated scan of photo metadata without downloading photo
   content;
3. bounded thumbnail previews;
4. local MCP-managed photo tags and descriptions for both personal and
   work/school accounts;
5. Graph-native photo descriptions where supported by OneDrive Personal;
6. OneDrive Personal album discovery and management; and
7. optional recycle-bin deletion of individual photos and files.

Microsoft Graph does not provide a general writable `tags` property for
OneDrive `driveItem` photos. It also exposes the `description` property and
album bundles only for OneDrive Personal. To avoid overstating Graph
capabilities, cross-account tags and descriptions will be stored as clearly
labelled local MCP annotations. Native descriptions and albums will be
available only when Graph reports a personal drive.

The change will not upload files, download original file content, edit image
bytes or EXIF data, delete folders, or permanently erase items.

## Motivation and Background

The server already authenticates Microsoft accounts for calendar and mail but
cannot find files or photos stored in those accounts' OneDrives. Users need to
search for files, discover all their photos, inspect camera metadata, preview a
selected image, organize personal photos into albums, and optionally remove an
individual file without establishing another account system.

Photo discovery must not require content download. Microsoft Graph represents
images using the `image` facet and camera photos using the `photo` facet on a
`driveItem`. A drive delta traversal returns these facets and other item
metadata without calling the `/content` endpoint. The traversal can therefore
enumerate photo metadata across the whole drive while keeping image bytes out
of the server.

Write access needs a stricter design than a single `off|read|write` mode.
Album management, local annotation, native description updates, and file
deletion have different risk and compatibility profiles. As with mail, the
server must request and expose only the capability classes configured for each
account.

## Current State

The repository currently has four aggregate domain tools:

- `calendar`;
- `mail`;
- `account`; and
- `system`.

Authentication, token caching, account selection, scope union, read-only
middleware, audit logging, observability, output tiering, and per-account mail
policies already provide patterns the new domain can follow.

There is no drive policy, `Files.Read` or `Files.ReadWrite` scope calculation,
drive metadata client, photo annotation store, or `drive` aggregate tool.

## Microsoft Graph Capability Constraints

The implementation must preserve the following distinction between what Graph
stores and what this MCP server stores.

| Capability | Personal OneDrive | Work/school OneDrive | Storage |
|---|---:|---:|---|
| File and photo metadata | Yes | Yes | Microsoft Graph |
| Whole-drive metadata traversal | Yes | Yes | Microsoft Graph |
| Thumbnail preview | Yes | Yes | Microsoft Graph |
| General photo tags | No Graph field | No Graph field | Local MCP annotations |
| Local photo description | Yes | Yes | Local MCP annotations |
| Native `driveItem.description` | Yes | No | Microsoft Graph |
| List and read albums | Yes | No | Microsoft Graph bundle |
| Create and manage albums | Yes | No | Microsoft Graph bundle |
| Move an individual file to recycle bin | Yes | Yes | Microsoft Graph |

The MCP interface must call locally stored values `localTags` and
`localDescription`. It must call the Graph value `nativeDescription`. It must
not present local annotations as though they were embedded in the image,
written to EXIF/IPTC metadata, or visible in OneDrive and Microsoft Photos.

## Proposed Change

### Aggregate Domain

Register one new top-level MCP tool:

```text
drive(operation=..., account=..., ...)
```

All functionality is dispatched through registered verbs. No verb will be
registered as a separate top-level MCP tool.

### Operation Registry

The initial registry will contain:

| Operation | Kind | Purpose |
|---|---|---|
| `help` | read | Describe available operations, gates, account compatibility, and per-verb annotations |
| `search_items` | read | Search the selected drive for files and folders |
| `list_items` | read | List a folder's direct children |
| `get_item` | read | Return metadata for one item |
| `list_photos` | read | List photos under a folder or special photo folder |
| `scan_photo_metadata` | read | Traverse all drive metadata and return only image/photo items without downloading content |
| `view_photo` | read | Return one bounded Graph thumbnail as MCP image content |
| `get_photo_annotations` | read | Read local tags and local description for a photo |
| `set_photo_annotations` | write | Replace local tags and/or local description for a photo |
| `set_native_description` | write | Update a personal-drive photo's Graph-native description |
| `list_albums` | read | List personal OneDrive albums |
| `get_album` | read | Return one personal album and paginated members |
| `create_album` | write | Create a personal OneDrive album |
| `update_album` | write | Rename a personal album or change its cover |
| `add_to_album` | write | Add an existing personal-drive photo to an album |
| `remove_from_album` | write | Remove a photo from an album without deleting the photo |
| `delete_album` | write | Delete an album bundle without deleting its member photos |
| `trash_item` | write | Move one non-folder file or photo to the OneDrive recycle bin |

### Per-Account Drive Policy

Replace a cumulative `off|read|manage` mode with independent action flags:

```go
type DrivePolicy struct {
    MetadataRead     bool
    Preview          bool
    Annotate         bool
    NativeDescribe   bool
    AlbumRead        bool
    AlbumManage      bool
    Trash            bool
}
```

The names above are conceptual. The implementation may use existing repository
naming conventions, but it must preserve independent authorization decisions.

Default values for every existing and newly authenticated account are `false`.
No drive verb other than `help` is exposed to an account until its relevant
flag is explicitly enabled.

The policy-to-operation matrix is:

| Policy flag | Authorized operations |
|---|---|
| `metadata_read` | `search_items`, `list_items`, `get_item`, `list_photos`, `scan_photo_metadata`, `get_photo_annotations` |
| `preview` | `view_photo` |
| `annotate` | `get_photo_annotations`, `set_photo_annotations` |
| `native_describe` | `set_native_description` |
| `album_read` | `list_albums`, `get_album` |
| `album_manage` | `list_albums`, `get_album`, `create_album`, `update_album`, `add_to_album`, `remove_from_album`, `delete_album` |
| `trash` | `trash_item` |

`album_manage` includes album reads because an agent must inspect an album
before safely mutating it. No other flag implies another flag.

### Delegated Scope Calculation

The account's requested Microsoft Graph scopes will be the union of its enabled
capabilities:

| Enabled action | Required delegated scope |
|---|---|
| metadata read | `Files.Read` |
| preview | `Files.Read` |
| local annotations | `Files.Read` for item identity/type validation |
| album read | `Files.Read` |
| native description write | `Files.ReadWrite` |
| album management | `Files.ReadWrite` |
| recycle-bin deletion | `Files.ReadWrite` |

When `Files.ReadWrite` is required, the scope union must not redundantly request
`Files.Read`. Accounts with no enabled drive actions must request neither file
scope.

Changing an account from a read scope to a write scope may require
reauthentication and renewed consent. Disabling the last write action must
reduce the requested scope on the next authentication lifecycle; it cannot
revoke a grant already held at Microsoft's authorization service.

### Full Photo Metadata Scan

`scan_photo_metadata` will use the selected account's drive-root delta
traversal. It will:

1. request metadata only with a bounded `$select` field set;
2. follow Graph paging without calling `/content` or a thumbnail endpoint;
3. locally retain only items with an `image` or `photo` facet;
4. return one bounded result page per MCP invocation;
5. return a signed, opaque, account-bound cursor for the next page;
6. mark the traversal complete when Graph returns a delta link; and
7. optionally return a signed delta cursor for a later incremental rescan.

This approach guarantees that completing the pagination traverses metadata for
the whole drive. It does not maintain a background full-drive index or
guarantee a stable snapshot while the drive is being modified.

Each photo metadata record will intentionally include:

- signed item reference;
- drive and item identity in `summary` and `raw` modes;
- name and parent-relative path;
- MIME type, size, created time, and modified time;
- width and height from the image facet;
- available camera make, model, exposure, aperture, focal length, ISO,
  orientation, and taken time from the photo facet;
- native description when Graph returns it;
- local tags and local description in `text` and `summary` modes;
- eTag and deletion state in `raw` mode; and
- whether a thumbnail preview is available.

Graph returns fewer photo-facet fields for work/school drives than for personal
drives. Missing values must remain absent; the server must not infer them from
filenames or download the photo to extract EXIF.

### Signed References and Cursors

Drive item, album, page, and delta references returned by tools must be opaque,
tamper-evident, and bound to:

- the authenticated account ID;
- the drive ID;
- the resource ID or Graph continuation state;
- the reference kind; and
- an expiry time where replay is unsafe.

Handlers must reject:

- a reference created for another account;
- a photo reference used as an album reference;
- malformed or modified references;
- expired continuation references; and
- a raw Graph URL supplied in place of a server-issued cursor.

Graph `@odata.nextLink` and `@odata.deltaLink` values must never be accepted
directly from MCP callers.

### Local Photo Annotations

Because Microsoft Graph has no general writable photo-tag property,
`set_photo_annotations` will maintain a server-local sidecar store keyed by:

```text
stable account ID + drive ID + drive item ID
```

An annotation record contains:

```text
localTags
localDescription
updatedAt
```

Rules for the store:

- only image/photo drive items may receive annotations;
- the handler must validate the item through Graph before the first write;
- tags are trimmed, non-empty, case-preserving, and case-insensitively unique;
- a record may hold at most 50 tags;
- an individual tag may contain at most 64 Unicode characters;
- a local description may contain at most 4,000 Unicode characters;
- an empty tag list and empty description delete the annotation record;
- writes must be atomic and safe across concurrent MCP calls;
- the file must use user-only operating-system permissions where supported;
- audit and application logs must never contain annotation values;
- annotations must be retained when an item is moved to the recycle bin so they
  remain available if the user restores it; and
- an annotation whose item no longer resolves must be reported as orphaned,
  not silently reassigned to another item.

The sidecar store is local to this server installation. It is not synchronized
between machines and is not visible in OneDrive clients. Backup and migration
of the sidecar are outside this change.

### Native Descriptions

`set_native_description` will use a Graph `driveItem` update only when the
selected drive is `personal`. It will:

- accept a photo reference, description, and optional expected eTag;
- verify the item is an image/photo;
- reject work/school drives before issuing a write request;
- use `If-Match` when an expected eTag is provided;
- return the updated item ID, description, and eTag; and
- state that the description is OneDrive-native.

Work/school users can still use `set_photo_annotations` for a local
description. The server must not silently redirect a requested native write to
the local store.

### Albums

Microsoft Graph models OneDrive Personal albums as bundles. Album operations
will therefore be enabled only when `/me/drive` reports a personal drive.

Album behavior:

- `list_albums` lists bundle metadata without downloading member content;
- `get_album` pages album members and returns photo metadata;
- `create_album` creates an empty album or an album with explicitly referenced
  existing photos, depending on Graph support;
- `update_album` may rename the album or select an existing member as cover;
- `add_to_album` and `remove_from_album` change membership without moving or
  deleting the underlying photo;
- `delete_album` removes the bundle only and must return confirmation that
  member photos were not deleted; and
- every member reference must belong to the same account and drive as the
  album.

For work/school drives, album operations must return a typed
`unsupported_for_drive_type` error before a bundle mutation is attempted.

### Recycle-Bin Deletion

`trash_item` will use the standard Graph DriveItem delete operation, which moves
the item to the OneDrive recycle bin.

The first release will intentionally limit deletion:

- exactly one item per call;
- files and photos only;
- no folders;
- no album bundles;
- no wildcard, search-result, or bulk deletion;
- no permanent delete operation;
- no automatic purge of local annotations;
- a fresh metadata lookup immediately before deletion;
- an `If-Match` precondition using the resolved eTag; and
- a confirmation containing account alias, item name, path, item ID, and the
  fact that the item was moved to the recycle bin.

The `trash` flag authorizes the agent to perform this reversible action without
a second `confirm=true` parameter. Permanently deleting an item would be a
separate future change requiring its own policy flag and human elicitation.

### Output Tiers

Every read operation will implement:

- `text` as the default;
- `summary` as deliberately curated JSON; and
- `raw` as complete Graph resource JSON where the operation maps directly to a
  Graph response.

For locally composed operations:

- `scan_photo_metadata` raw output will contain complete, unmodified individual
  Graph `driveItem` objects inside a server page envelope with opaque cursors;
- `get_photo_annotations` raw output will contain the complete local
  annotation record; and
- Graph raw items will not be modified to inject local annotation fields.

Text and summary modes may compose Graph metadata and local annotations because
their schemas are intentionally server-defined.

Write operations return text confirmations only and do not accept `output`.

Original content remains unavailable. `view_photo` returns a bounded thumbnail
as MCP image content plus a short text metadata block.

### Aggregate MCP Annotations

Because the `drive` domain contains both read and write verbs, its conservative
aggregate annotations will be:

| Annotation | Value | Reason |
|---|---:|---|
| title | `OneDrive Files and Photos` | Human-readable picker name |
| read-only | `false` | Annotation, album, description, and trash verbs write |
| destructive | `true` | Album deletion and recycle-bin deletion remove resources |
| idempotent | `false` | Album creation is non-idempotent |
| open-world | `true` | Most verbs call Microsoft Graph |

Every registry verb must additionally document its own annotation semantics in
`drive(operation="help")`.

## Detailed Requirements

### Functional Requirements

1. The server MUST register exactly one new aggregate MCP tool named `drive`.
2. The `drive` tool MUST dispatch only registered operation verbs.
3. The `drive` tool MUST provide an `operation="help"` verb.
4. All drive actions MUST resolve an account through the existing account
   resolver.
5. All drive actions MUST enforce the selected account's drive policy before
   making a Graph or local annotation call.
6. Every drive policy flag MUST default to disabled.
7. Drive policy flags MUST be independently configurable per account.
8. The account status response MUST report enabled drive action classes and
   effective Graph file scope without exposing tokens.
9. Scope calculation MUST request no file scope when all drive flags are
   disabled.
10. Scope calculation MUST request `Files.Read` when only read or local
    annotation actions require Graph access.
11. Scope calculation MUST request `Files.ReadWrite` when any Graph write
    action is enabled.
12. Scope calculation MUST remove redundant `Files.Read` when
    `Files.ReadWrite` is present.
13. Work/school accounts MUST be supported for metadata, preview, local
    annotations, and recycle-bin deletion.
14. Personal accounts MUST additionally support native descriptions and
    albums.
15. The server MUST determine drive compatibility from Graph drive metadata,
    not from email address shape.
16. `search_items` MUST support a bounded query, account selection,
    pagination, and all three output tiers.
17. `list_items` MUST list direct children of a referenced folder without
    recursively downloading or traversing content.
18. `get_item` MUST return metadata without returning original file content.
19. `list_photos` MUST identify photos through Graph facets or supported MIME
    metadata, not only filename extensions.
20. `scan_photo_metadata` MUST traverse drive metadata through the Graph delta
    API.
21. `scan_photo_metadata` MUST NOT call a content or thumbnail endpoint.
22. `scan_photo_metadata` MUST return a bounded page and an opaque continuation
    cursor when more Graph pages remain.
23. Completing `scan_photo_metadata` pagination MUST visit metadata from every
    drive delta page reachable from the initial root traversal.
24. The final scan page MUST indicate completion and MAY return an opaque delta
    cursor for later incremental scanning.
25. Page and delta cursors MUST be signed and bound to the account and drive.
26. Raw Graph continuation URLs MUST NOT be accepted as MCP input.
27. `view_photo` MUST retrieve only a bounded Graph thumbnail.
28. `view_photo` MUST reject non-image items before requesting thumbnail
    content.
29. Thumbnail size, media type, and response byte limits MUST be validated
    before emitting MCP image content.
30. Original file-content download MUST NOT be exposed by any operation in
    this change.
31. `get_photo_annotations` MUST return only annotations associated with the
    selected account, drive, and item.
32. `set_photo_annotations` MUST reject non-photo items.
33. `set_photo_annotations` MUST enforce tag and description limits before
    writing.
34. Local annotation writes MUST be atomic.
35. Logs and audit attributes MUST NOT contain tag or description values.
36. Local annotations MUST be labelled as local in every output mode.
37. `set_native_description` MUST be rejected for work/school drives.
38. `set_native_description` MUST NOT fall back to a local annotation.
39. `set_native_description` MUST use an optional caller-provided eTag as an
    `If-Match` precondition.
40. Album operations MUST be rejected for work/school drives before a Graph
    bundle mutation is attempted.
41. Album member references MUST resolve to the same account and drive as the
    album.
42. `remove_from_album` MUST NOT delete, move, or trash the underlying photo.
43. `delete_album` MUST NOT delete, move, or trash member photos.
44. `trash_item` MUST require the account's `trash` flag.
45. `trash_item` MUST reject folder and album references.
46. `trash_item` MUST resolve fresh metadata and use an eTag precondition.
47. `trash_item` MUST move only one item to the recycle bin per call.
48. No operation in this change MUST permanently delete a drive item.
49. All write operations MUST pass through the global read-only guard.
50. All operations MUST pass through observability and audit middleware.
51. Audit identity MUST use `{domain}.{operation}`, such as
    `drive.scan_photo_metadata`.
52. Read operations MUST implement `text`, `summary`, and `raw`.
53. Read operations MUST default to `text`.
54. Write operations MUST return text confirmations without an `output`
    parameter.
55. The `drive` aggregate tool MUST explicitly set all five MCP annotations.
56. Every drive verb MUST have a per-verb annotation assertion.
57. Tool descriptions and examples MUST be owned by the drive verb registry.
58. `extension/manifest.json` MUST include the `drive` aggregate tool.
59. The CRUD test prompt MUST cover enabled, disabled, supported, and
    unsupported drive behaviors.
60. The CRUD benchmark harness and CSV schema MUST add a `mcp_drive` bucket
    because this change introduces a top-level domain.

### Non-Functional Requirements

1. Metadata-only scans MUST not buffer the entire drive in memory.
2. Collection page sizes MUST have conservative defaults and hard maximums.
3. Thumbnail payloads MUST have a hard byte ceiling.
4. All Graph calls MUST use the existing retry, timeout, error translation,
   serialization, and sanitization infrastructure.
5. Signed references MUST use a process-owned secret and constant-time
   signature verification.
6. The annotation store MUST tolerate interrupted writes without corrupting
   previously committed records.
7. The annotation store MUST not require a database service.
8. The design MUST remain compatible with existing accounts whose persisted
   records do not contain drive policy fields.
9. New code MUST be split into small, single-purpose files under appropriate
   `internal/` packages.
10. All packages, functions, methods, structs, interfaces, and exported fields
    MUST follow the repository's Go documentation standard.
11. The completed implementation MUST pass `make ci`.

## Tool Contract Sketch

The following examples illustrate the intended public contract. Exact Go SDK
option names may differ.

```text
drive(
  operation="scan_photo_metadata",
  account="personal",
  cursor="<optional signed cursor>",
  limit=50,
  output="text"
)
```

```text
1. IMG_2041.HEIC
   Path: /Pictures/Camera Roll/IMG_2041.HEIC
   Taken: 2026-07-11T03:40:19Z
   Dimensions: 4032 x 3024
   Camera: Apple iPhone
   Local tags: family, beach
   Local description: Walking near the headland
   Native description: Summer holiday
   Preview available: yes
   Ref: <signed item reference>

Returned: 1
Scan complete: no
Next cursor: <signed cursor>
```

```text
drive(
  operation="set_photo_annotations",
  account="personal",
  item_ref="<signed item reference>",
  local_tags=["family", "beach"],
  local_description="Walking near the headland"
)
```

```text
Updated local photo annotations
Account: personal
Item: IMG_2041.HEIC
Tags: 2
Local description: set
Storage: this MCP server only
```

```text
drive(
  operation="create_album",
  account="personal",
  name="Coast trip"
)
```

```text
Created OneDrive album
Account: personal
Album: Coast trip
Album ID: <id>
Members: 0
```

```text
drive(
  operation="trash_item",
  account="personal",
  item_ref="<signed item reference>"
)
```

```text
Moved OneDrive item to recycle bin
Account: personal
Item: IMG_2041.HEIC
Path: /Pictures/Camera Roll/IMG_2041.HEIC
Item ID: <id>
Local annotations retained: yes
```

## Architecture and Implementation Plan

### Package Boundaries

Implementation should use small files and narrow interfaces:

```text
internal/
  auth/
    drive_policy.go
    drive_scopes.go
  drive/
    doc.go
    client.go
    drive_type.go
    item_ref.go
    cursor.go
    photo_metadata.go
    album.go
  driveannotations/
    doc.go
    record.go
    store.go
    file_store.go
    validation.go
  tools/
    drive_registry.go
    drive_help.go
    drive_search.go
    drive_list.go
    drive_get.go
    drive_photo_list.go
    drive_photo_scan.go
    drive_photo_view.go
    drive_photo_annotations_get.go
    drive_photo_annotations_set.go
    drive_native_description_set.go
    drive_album_list.go
    drive_album_get.go
    drive_album_create.go
    drive_album_update.go
    drive_album_add.go
    drive_album_remove.go
    drive_album_delete.go
    drive_item_trash.go
    drive_text_format.go
  server/
    drive_verbs.go
    drive_target_guards.go
```

This is a directional layout, not a requirement to create empty abstractions.
Each file should own one operation or one cohesive primitive.

### Interfaces

Handlers should depend on narrow collaborators rather than a broad Graph
client:

```go
type PhotoMetadataScanner interface {
    ScanPhotoMetadata(
        ctx context.Context,
        account auth.Account,
        cursor string,
        limit int,
    ) (PhotoMetadataPage, error)
}

type PhotoAnnotationStore interface {
    Get(ctx context.Context, key AnnotationKey) (Annotation, error)
    Put(ctx context.Context, key AnnotationKey, value Annotation) error
    Delete(ctx context.Context, key AnnotationKey) error
}

type AlbumManager interface {
    CreateAlbum(ctx context.Context, account auth.Account, name string) (Album, error)
    AddItem(ctx context.Context, account auth.Account, albumID, itemID string) error
    RemoveItem(ctx context.Context, account auth.Account, albumID, itemID string) error
}
```

Production adapters call the pinned Microsoft Graph SDK. Unit tests use narrow
fakes that assert authorization and call ordering.

### Middleware Order

The `drive` domain must follow the existing server chain:

1. parse and validate operation;
2. resolve account;
3. apply per-account drive target guard;
4. apply the global read-only guard for write verbs;
5. attach observability;
6. attach audit logging; and
7. call the operation handler.

Policy denial and unsupported-drive errors must occur before the first
mutation. Where a read is necessary to establish drive type, it may occur
before an unsupported error but no Graph write may occur.

### Configuration and Persistence

The account record schema will gain drive policy fields with false-valued
backward-compatible defaults.

The local annotation store path will be configurable, with a default adjacent
to the server's existing user-scoped state. The implementation must document:

- that the file contains user-authored metadata;
- that it should be included in backup if annotations matter;
- that deleting the file removes local annotations only; and
- that the file must not be placed inside the repository by default.

### Documentation Placement

Per repository governance:

- per-verb parameters, examples, compatibility, and response behavior belong
  in the drive registry;
- the account capability model, local annotation distinction, scope behavior,
  and personal-versus-business limitations belong in `docs/concepts.md`;
- first-run drive configuration and reauthentication belong in
  `docs/quickstart.md`;
- consent, unsupported album, corrupt annotation store, cursor, and thumbnail
  failures belong in `docs/troubleshooting.md`;
- implementation internals belong in `docs/reference/architecture.md` or a
  dedicated reference page; and
- the root README receives only a landing-page link and fifth-domain example
  if required.

## Delivery Phases

### Phase 1: Policy and Scope Foundation

- add the independent drive policy;
- add backward-compatible account persistence;
- calculate `Files.Read` and `Files.ReadWrite` unions;
- surface effective drive capability status;
- add policy and scope unit tests; and
- document consent transitions.

### Phase 2: Read-Only Drive and Photo Metadata

- register the `drive` aggregate and help operation;
- implement signed item and page references;
- implement search, list, get, photo list, and complete metadata scan;
- implement output tiers and text formatters;
- add account-crossing and cursor-tampering tests; and
- verify that metadata scans make no content request.

### Phase 3: Thumbnail Preview

- implement bounded thumbnail selection and validation;
- emit MCP image content;
- reject non-photo items and oversized or invalid media;
- add content-leakage and payload-limit tests.

### Phase 4: Local Photo Annotations

- implement the atomic sidecar store;
- implement get and set operations;
- merge clearly labelled local fields into text and summary metadata;
- add validation, redaction, concurrency, and corruption tests.

### Phase 5: Personal Native Descriptions and Albums

- detect and cache drive type safely;
- implement native description updates for personal drives;
- implement album list, detail, create, update, membership, and delete;
- reject these operations for work/school drives;
- verify that album member removal and album deletion preserve photos.

### Phase 6: Gated Recycle-Bin Deletion

- implement single-file trash with fresh metadata and eTag precondition;
- reject folders, albums, bulk input, and stale items;
- retain local annotations;
- add audit assertions that exclude names, paths, tags, and descriptions where
  the sanitization policy requires masking.

### Phase 7: Integration and Documentation

- update the extension manifest;
- update CRUD prompt, harness bucket, and benchmark CSV header;
- update embedded user documentation;
- run live personal and work/school account smoke tests;
- run `make ci`; and
- create the implementation PR through the normal review process.

Each phase should be independently reviewable. Graph write phases must not
begin until policy and scope tests pass.

## Test Plan

### Unit Tests

- drive policy defaulting and persistence migration;
- exact policy-to-operation authorization matrix;
- scope union and downscoping;
- aggregate and per-verb annotations;
- drive-type compatibility;
- signed item, album, next-page, and delta cursor validation;
- cross-account and cross-drive reference rejection;
- delta page filtering for `image` and `photo` facets;
- scan completion and incremental cursor behavior;
- proof that scan handlers never call content or thumbnail clients;
- photo metadata summary and text formatting;
- thumbnail MIME and byte limits;
- annotation tag normalization and limits;
- atomic annotation replacement;
- concurrent annotation updates;
- corrupt store recovery behavior;
- annotation redaction from logs and audit;
- personal native description update;
- work/school native description rejection before write;
- album membership identity checks;
- album deletion preserving item calls;
- trash policy denial;
- folder and bundle trash rejection;
- eTag conflict translation;
- read-only guard coverage for every write verb; and
- text, summary, and raw output tier behavior.

### Integration Tests

Use one personal Microsoft account and, where available, one work/school
account:

1. authenticate with all drive flags disabled and confirm no file scope;
2. enable metadata read and confirm `Files.Read`;
3. finish a full photo metadata scan and confirm no original content is
   returned;
4. preview a selected photo thumbnail;
5. write and read local annotations;
6. confirm local annotations are not represented as Graph-native;
7. enable a Graph write action and complete `Files.ReadWrite` consent;
8. update a native description on a personal photo;
9. confirm the same operation is unsupported on a work/school photo;
10. create a personal album, add and remove a photo, update the album, then
    delete the album;
11. confirm the member photo still exists after album deletion;
12. confirm albums are unsupported on work/school OneDrive;
13. trash one test file and confirm it appears in the recycle bin;
14. confirm its local annotations remain in the sidecar;
15. disable the trash flag and confirm the verb is blocked before Graph; and
16. enable global read-only mode and confirm all drive write verbs are blocked.

### Security Tests

- forged reference;
- expired page cursor;
- reference replay against another account;
- injected raw continuation URL;
- non-photo annotation request;
- annotation path traversal attempt;
- tag and description oversize input;
- annotation content absent from logs, spans, and audit attributes;
- work/school album mutation attempt;
- folder deletion attempt;
- stale eTag deletion attempt;
- deletion request without the trash flag;
- original-content endpoint invocation assertion; and
- token and Graph URL leakage checks.

## Acceptance Criteria

### AC1: Drive is disabled by default

```gherkin
Given an existing account record with no drive policy fields
When the server loads the account
Then every drive policy flag is disabled
And no Microsoft Graph file scope is requested
And all drive operations except help are denied for that account
```

### AC2: Photo metadata does not download content

```gherkin
Given an account with metadata_read enabled
And a drive containing photos across multiple folders and Graph pages
When the caller follows scan_photo_metadata until scan complete is true
Then metadata for every photo item returned by the drive traversal is emitted
And no photo content endpoint is called
And no thumbnail endpoint is called
```

### AC3: Cursors are account-bound

```gherkin
Given a scan cursor issued for account A
When the caller supplies it while selecting account B
Then the server rejects the cursor
And no Graph request using the cursor is made
```

### AC4: Work/school metadata remains partial but honest

```gherkin
Given a work or school drive whose photo facet contains only takenDateTime
When the caller requests photo metadata
Then the available taken time is returned
And unavailable camera fields are absent
And the server does not download the photo to infer missing EXIF
```

### AC5: Tags are clearly local

```gherkin
Given an image item and an account with annotate enabled
When the caller sets local tags and a local description
Then the values are written atomically to the local annotation store
And later text and summary metadata label the values as local
And no Graph update request is made
And the response states that the annotations are not synchronized to OneDrive
```

### AC6: Annotation content is private

```gherkin
Given a local tag and description containing sensitive text
When the set and get annotation operations complete
Then the values are absent from application logs
And the values are absent from audit attributes
And the values are absent from tracing attributes
```

### AC7: Native descriptions are personal-only

```gherkin
Given a work or school drive
And native_describe is enabled
When the caller invokes set_native_description
Then the server returns unsupported_for_drive_type
And no Graph write request is made
And no local annotation is written
```

### AC8: Album management is personal-only

```gherkin
Given a work or school drive
And album_manage is enabled
When the caller invokes create_album
Then the server returns unsupported_for_drive_type
And no Graph bundle mutation is attempted
```

### AC9: Removing an album member preserves the photo

```gherkin
Given a personal album containing an existing photo
When remove_from_album succeeds
Then the photo is no longer an album member
And the underlying drive item still exists at its original location
```

### AC10: Deleting an album preserves its photos

```gherkin
Given a personal album containing existing photos
When delete_album succeeds
Then the album bundle no longer exists
And every former member photo remains in the drive
And the confirmation states that member photos were not deleted
```

### AC11: Trash exposure is independent

```gherkin
Given an account with metadata_read enabled and trash disabled
When the caller invokes trash_item with a valid file reference
Then the server denies the operation
And no Graph delete request is made
```

### AC12: Trash is single-item and recoverable

```gherkin
Given an account with trash enabled
And a valid non-folder file reference
When trash_item succeeds
Then exactly one Graph DriveItem delete request is made with an eTag precondition
And the item is moved to the OneDrive recycle bin
And any local annotations are retained
And no permanent-delete request is made
```

### AC13: Folder deletion is not exposed

```gherkin
Given an account with trash enabled
And a folder reference
When the caller invokes trash_item
Then the server rejects the request before deletion
And no child or folder is moved to the recycle bin
```

### AC14: Global read-only remains authoritative

```gherkin
Given an account whose write flags are enabled
And the server is running in global read-only mode
When the caller invokes any drive write operation
Then the read-only guard denies it
And neither Graph nor the annotation store is mutated
```

### AC15: Scope follows exposed actions

```gherkin
Given an account with metadata_read and annotate enabled
And every Graph write action disabled
When authentication scopes are calculated
Then Files.Read is requested
And Files.ReadWrite is not requested

Given the same account with trash enabled
When authentication scopes are calculated
Then Files.ReadWrite is requested
And redundant Files.Read is omitted
```

### AC16: Manifest and harness remain synchronized

```gherkin
Given the drive aggregate is registered
When repository consistency tests run
Then extension/manifest.json contains the drive tool
And the CRUD test prompt exercises drive operations
And the benchmark harness and CSV header contain mcp_drive
```

## Risks and Mitigations

| Risk | Impact | Mitigation |
|---|---|---|
| Users assume local tags are in OneDrive | Data appears missing in other apps | Distinct field names, explicit confirmations, concepts documentation |
| Local annotations contain sensitive text | Privacy exposure | User-only permissions, atomic local file, sanitization, no content in logs/audit |
| Whole-drive scans are slow on large drives | Timeouts and high Graph use | Bounded pages, opaque continuation, delta cursor, no in-memory full scan |
| A drive changes during traversal | Results are not a point-in-time snapshot | Document delta semantics and support later incremental reconciliation |
| Work/school photo metadata is sparse | Inconsistent user experience | Return only Graph-provided fields and document compatibility |
| `Files.ReadWrite` is broader than a single action | Increased consent risk | Independent local flags, request scope only for enabled actions, guard every verb |
| Album mutation accidentally affects photos | Potential data loss | Same-drive checks, operation-specific tests, confirm bundle-only behavior |
| A stale reference deletes the wrong version | Data loss | Fresh lookup and eTag precondition |
| Folder deletion removes a tree | Large data loss | Reject folders and bulk deletion in this release |
| Sidecar and drive state diverge | Orphaned annotations | Stable composite keys, preserve and report orphan state, never reassign silently |

## Alternatives Considered

### Use a Single `off|read|write` Drive Mode

Rejected. It would expose album management and deletion whenever a user only
wanted native description edits, and it would not match the independent mail
capability model.

### Store Tags in a Graph DriveItem Property

Rejected because Microsoft Graph v1.0 does not expose a general writable photo
tags field on `driveItem`.

### Modify EXIF or IPTC Metadata

Rejected. It would require original-content download, binary rewriting,
re-upload, format-specific behavior, and much broader data-loss handling.

### Use SharePoint Custom Columns for Work/School Tags

Rejected for the initial release. It lacks personal-drive parity, depends on
library configuration, and would introduce site/list permissions and semantics
beyond a OneDrive file tool.

### Pretend Local Descriptions Are Native Descriptions

Rejected. The storage distinction is material to portability and must be
visible to the user and agent.

### Build a Persistent Full-Drive Photo Index

Rejected. A complete delta traversal satisfies metadata enumeration without a
background database, synchronization daemon, or stale-index lifecycle.

### Support Permanent Deletion

Rejected for this change. Recycle-bin deletion covers the requested removable
workflow while remaining recoverable. Permanent erasure needs a separate gate,
human elicitation, and explicit governance review.

### Support Albums on Work/School Accounts Through Folder Emulation

Rejected. A folder is not an album and would change item location. The server
must return an honest unsupported response instead.

## Rollback Plan

1. Disable all drive policy flags in account records.
2. Stop registering the `drive` aggregate tool and remove it from the extension
   manifest in a coordinated rollback.
3. Remove file scopes from future authentication scope calculation.
4. Preserve the local annotation sidecar unless the user explicitly chooses to
   delete it.
5. Do not attempt to undo user-created albums, native descriptions, or
   recycle-bin moves automatically.
6. Document that Microsoft consent grants may remain until revoked by the user
   in Microsoft account or tenant administration.

Rollback must never delete local annotations or Graph resources as a side
effect.

## Estimated Effort

| Workstream | Estimate |
|---|---:|
| Policy, persistence, and scope union | 1.5-2 days |
| Drive read/search/list and signed references | 2-3 days |
| Full photo metadata scan and output tiers | 2-3 days |
| Thumbnail preview | 1-1.5 days |
| Local annotation store and tools | 2-3 days |
| Native descriptions and personal albums | 2.5-4 days |
| Gated recycle-bin deletion | 1-1.5 days |
| Documentation, harness, integration, and hardening | 2-3 days |
| **Total** | **14-21 engineering days** |

The broad range reflects the need to validate Graph bundle behavior against a
real personal OneDrive and compatibility behavior against a work/school drive.

## Decision

Pending maintainer approval.

Recommended decision:

- implement the read-only metadata foundation first;
- use independent per-account action flags;
- store cross-account tags and descriptions as explicitly local annotations;
- expose native descriptions and albums only for personal OneDrive;
- interpret delete as a single-file move to the recycle bin; and
- leave permanent erase, folders, upload, original download, EXIF editing,
  sharing, and folder-based album emulation out of scope.

## References

- [Microsoft Graph DriveItem resource](https://learn.microsoft.com/en-us/graph/api/resources/driveitem?view=graph-rest-1.0)
- [Microsoft Graph photo facet](https://learn.microsoft.com/en-us/graph/api/resources/photo?view=graph-rest-1.0)
- [Track changes for a DriveItem hierarchy](https://learn.microsoft.com/en-us/graph/api/driveitem-delta?view=graph-rest-1.0)
- [Update a DriveItem](https://learn.microsoft.com/en-us/graph/api/driveitem-update?view=graph-rest-1.0)
- [Microsoft Graph bundle resource](https://learn.microsoft.com/en-us/graph/api/resources/bundle?view=graph-rest-1.0)
- [Create a bundle](https://learn.microsoft.com/en-us/graph/api/drive-post-bundles?view=graph-rest-1.0)
- [Add an item to a bundle](https://learn.microsoft.com/en-us/graph/api/bundle-additem?view=graph-rest-1.0)
- [Update a bundle](https://learn.microsoft.com/en-us/graph/api/bundle-update?view=graph-rest-1.0)
- [Delete a DriveItem](https://learn.microsoft.com/en-us/graph/api/driveitem-delete?view=graph-rest-1.0)
- `docs/cr/CR-0052-mcp-tool-annotations.md`
- `docs/cr/CR-0060-aggregate-domain-tools.md`
- `docs/cr/CR-0061-in-server-documentation-tools.md`
- `docs/concepts.md`
- `docs/reference/auth-flows.md`
