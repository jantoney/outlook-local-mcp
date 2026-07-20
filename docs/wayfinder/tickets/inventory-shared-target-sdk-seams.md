# Inventory the shared target SDK seams

Labels: `wayfinder:research`
Status: closed
Assignee: /root/research_sdk_seams

## Question

Where is the current code coupled to `client.Me()`, which installed Graph SDK
v1.96.0 request builders provide equivalent `Users().ByUserId(owner)` routes,
and what is the smallest deep-module seam that can support own and shared
targets without a broad abstraction leak?

## Evidence to verify

- All calendar and mail production call sites using `Me()`.
- Concrete SDK builder type differences between `Me()` and user-item routes.
- Existing test seams and mock HTTP patterns that can prove requested URLs.
- Opportunities for small domain-specific routing modules rather than one large interface.
- Operations whose SDK builder shapes prevent a uniform target abstraction.

## Resolution

The pinned `github.com/microsoftgraph/msgraph-sdk-go` v1.96.0 already has the
small routing seam required by CR-0067. `GraphServiceClient.Me()` and
`GraphServiceClient.Users().ByUserId(owner)` both return the **same concrete
type**, `*users.UserItemRequestBuilder`. `Me()` creates that builder with the
SDK's special `me-token-to-replace` path value, while `ByUserId` supplies the
configured owner as `user%2Did`; the shared builder's URL template is
`{+baseurl}/users/{user%2Did}`. Consequently, switching own versus shared
targets does not require raw URLs, a generated-builder facade, or a large
calendar/mail client interface.

The smallest deep-module seam is one target-to-user-builder function in the
Graph boundary, conceptually:

```go
func UserBuilder(client *msgraphsdk.GraphServiceClient, target ResourceTarget) *users.UserItemRequestBuilder {
	if target.IsShared() {
		return client.Users().ByUserId(target.Owner)
	}
	return client.Me()
}
```

Handlers can keep all existing typed chains after replacing their leading
`client.Me()` with this selected user builder. The `ResourceTarget` should
carry the already-resolved own/shared kind and configured owner; alias lookup,
profile authorization, and fixed-calendar policy belong before this SDK seam.
The builder helper must not accept a free-form owner from a tool request.

### Current coupling inventory

There are 42 executable `Me()` calls in 26 non-test files, plus two doc-comment
references. The resource call sites are:

- Calendar collections and items: `create_event.go` (primary and selected
  calendar), `delete_event.go`, `get_event.go`, `list_events.go` (primary and
  selected calendar views), `search_events.go`, `update_event.go`, and
  `reschedule_event.go` (read and patch).
- Calendar metadata and actions: `list_calendars.go`, `get_free_busy.go`,
  `cancel_meeting.go`, and `respond_event.go` (accept, tentative, decline).
- Mail collections, folders, and messages: `list_mail_folders.go`,
  `list_messages.go` (folder and mailbox), `search_messages.go` (folder and
  mailbox), `get_message.go`, and `get_conversation.go` (message and mailbox
  collection).
- Draft lifecycle: `create_draft.go`, `create_reply_draft.go` (reply/reply-all
  plus provenance patch), `create_forward_draft.go` (forward plus provenance
  patch), `update_draft.go` (patch plus verification read), `delete_draft.go`,
  and `send_draft.go` (send plus summary/attachment reads).
- Attachments: `add_attachment.go` (direct POST, upload-session creation, and
  verification), `list_attachments.go`, and `get_attachment.go`.
- `internal/auth/email_resolver.go` also calls `Me().Get()` to identify the
  signed-in principal. That call is intentionally **not** a resource-target
  seam: it must continue to resolve the delegate account, never the shared
  owner.

CR-0067 currently excludes meeting response and cancellation on behalf of an
owner. The inventory includes those calls so later implementation does not
accidentally make them shared merely through a mechanical replacement.

### Equivalent generated shared routes

Starting with `user := client.Users().ByUserId(owner)`, v1.96.0 exposes every
generated route needed by the currently in-scope operations:

- primary calendar and view: `user.Events()`, `user.CalendarView()`, and
  `user.Events().ByEventId(id)`;
- selected calendar: `user.Calendars().ByCalendarId(calendarID).Events()`,
  `.CalendarView()`, and event-item builders;
- mailbox and folder reads: `user.MailFolders()`,
  `.ByMailFolderId(folderID).Messages()`, and `user.Messages()`;
- messages, drafts, replies, forwards, and attachments:
  `user.Messages().ByMessageId(id)` with `Get`, `Patch`, `Delete`, `Send`,
  `CreateReply`, `CreateReplyAll`, `CreateForward`, and `Attachments`, including
  `CreateUploadSession`;
- direct send is also generated as `user.SendMail().Post(...)`, although
  CR-0067 deliberately permits only sending an existing shared draft.

Own and shared roots therefore have no concrete builder-type difference. The
important type differences are below the user root and already exist in the
current `/me` implementation: primary event collections/items differ from
`calendars/{calendar-id}` event collections/items; primary `CalendarView`
differs from a selected calendar's `CalendarView`; and mailbox-wide message
builders differ from folder-contained message builders. Those shapes prevent
one useful uniform "resource builder" interface across every operation, but
they do **not** prevent uniform own/shared target selection. Keep the existing
small per-handler primary/selected-calendar and mailbox/folder branches rather
than exporting a broad interface that mirrors the SDK.

### Test seam

`internal/tools/test_helpers_test.go` already constructs a real generated
`GraphServiceClient` over an anonymous Kiota adapter and an `httptest.Server`.
Its `testTransport.RoundTrip` rewrites Graph hosts to the local server while
preserving the generated request path. Existing tests such as
`create_reply_draft_test.go`, `create_forward_draft_test.go`, and
`respond_event_test.go` capture `r.URL.Path`; this same seam can assert exact
decoded paths such as `/users/shared@example.com/calendarView` and
`/users/shared@example.com/messages` (and inspect `EscapedPath` when encoded
octets matter) without credentials or SDK mocks. Tests should also assert that
an omitted shared target still emits `/me/...`, and that authorization/alias
failures make zero HTTP requests.

## Sources

- Repository production call sites: `internal/tools/*.go` and
  `internal/auth/email_resolver.go` (`rg --glob '*.go' --glob '!*_test.go'
  '\\.Me\\(\\)' internal`).
- Repository HTTP seam: `internal/tools/test_helpers_test.go`;
  representative path capture in `internal/tools/create_reply_draft_test.go`,
  `internal/tools/create_forward_draft_test.go`, and
  `internal/tools/respond_event_test.go`.
- Installed SDK entry points:
  `C:/Users/jayan/go/pkg/mod/github.com/microsoftgraph/msgraph-sdk-go@v1.96.0/graph_service_client.go`
  (`Me`) and `graph_base_service_client.go` (`Users`).
- Installed SDK user root:
  `C:/Users/jayan/go/pkg/mod/github.com/microsoftgraph/msgraph-sdk-go@v1.96.0/users/users_request_builder.go`
  (`ByUserId`) and `users/user_item_request_builder.go` (calendar, event,
  mail-folder, message, and send-mail child builders).
- Installed SDK generated route evidence under the same `users/` directory:
  `item_calendar_view_request_builder.go`,
  `item_calendars_item_calendar_view_request_builder.go`,
  `item_events_request_builder.go`,
  `item_calendars_item_events_request_builder.go`,
  `item_mail_folders_request_builder.go`,
  `item_mail_folders_item_messages_request_builder.go`,
  `item_messages_request_builder.go`,
  `item_messages_message_item_request_builder.go`, and
  `item_send_mail_request_builder.go`.
