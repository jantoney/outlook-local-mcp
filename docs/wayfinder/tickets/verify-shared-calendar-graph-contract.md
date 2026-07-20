# Verify the shared calendar Graph contract

Labels: `wayfinder:research`
Status: closed
Assignee: /root/research_shared_calendar

## Question

For every calendar operation included by CR-0067, which Microsoft Graph v1.0
owner routes, delegated scopes, sharing roles, ID rules, and personal-account
limitations are authoritative, and which operations must be narrowed or
excluded before implementation?

## Evidence to verify

- Direct owner access through `/users/{owner}/calendar...` and custom calendar routes.
- Read versus write behavior for sharees and delegates.
- Owner-view versus recipient-view calendar and event identifiers.
- Personal Microsoft account support for `Calendars.Read.Shared` and `Calendars.ReadWrite.Shared`.
- Meeting creation, organizer, notification, and delete semantics on shared calendars.

## Resolution

Microsoft's v1.0 shared-calendar contract supports CR-0067 only after the
following narrowing. The generic calendar API exposes many
`/users/{id}/calendars/{id}` routes, but the scenario-specific shared-calendar
guidance is authoritative about which mailbox view and IDs a delegated caller
may use.

### Authoritative route and ID model

There are two distinct access models and their identifiers cannot be mixed:

| Model | Authoritative routes | ID rule | CR-0067 decision |
|---|---|---|---|
| Owner-view shared or delegated **primary** calendar | The dedicated shared read guide documents `GET /users/{owner}/calendar`, `GET /users/{owner}/calendar/events`, and `GET /users/{owner}/calendar/events/{event-id}` | Calendar and event IDs belong to the owner's mailbox only. Using them against `/me` or the recipient's `/users/{recipient}` route fails. | Supported for reads. Although generic method pages expose `/users/{owner}` write path shapes, the dedicated `.Shared` walkthrough does not establish delegated owner-view writes. Do not claim those writes without a live contract test. |
| Recipient-view mounted shared/delegated calendar, including primary and **custom** calendars | Discover with `GET /me/calendars` (or `/users/{signed-in-recipient}/calendars`), then use `/me/calendars/{recipient-calendar-id}/events...` or `/calendarView...` | Calendar and event IDs belong to the local copy in the recipient's mailbox. They are not owner-view IDs. | This is the documented shared-write route and the documented custom-calendar route. It is not representable by CR-0067's universal `/users/{owner}` rule; add an explicit recipient-view target kind storing the mounted calendar ID. |

The shared-calendar guidance documents direct owner-mailbox reads with the
`calendar` shortcut for the owner's primary calendar. Its shared write
walkthrough posts to the recipient's mounted copy, and it documents custom
shared calendar access through that same recipient view. Therefore a generic
SDK route existing for `/users/{owner}/calendars/{id}` is not evidence that an
owner-view custom calendar ID or owner-view write is valid for a sharee using a
`.Shared` scope.

`list_calendars` must also be narrowed. Shared-calendar discovery is a
recipient-view operation (`GET /me/calendars`, optionally filtered/validated by
`owner.address`, `canEdit`, and the configured owner). It must not enumerate all
calendars owned by `/users/{owner}` or silently convert mounted IDs into
owner-view IDs. `GET /users/{owner}/calendar` is a single primary-calendar read,
not list discovery.

Event IDs are case-sensitive. Normal Outlook item IDs can change when an item
is moved; `Prefer: IdType="ImmutableId"` makes an event ID stable while it
remains in the same mailbox, but it does not make an owner-view ID usable in a
recipient mailbox (or vice versa). Calendar container IDs do not support the
immutable-ID preference, although Microsoft states their regular IDs are
already constant. The implementation should either consistently request
immutable event IDs or treat returned IDs as mailbox-view-scoped opaque values;
in both cases it must retain target metadata.

### Delegated scopes and Exchange sharing roles

OAuth permission is only an application ceiling. The owner must separately
share or delegate the calendar, and the effective calendar role controls what
Graph returns or permits:

| Local profile | OAuth scope | Required effective role/behavior |
|---|---|---|
| `read` | `Calendars.Read.Shared` | `freeBusyRead` exposes availability only; `limitedRead` also exposes titles and locations; `read` exposes non-private event details. A read profile cannot promise full event bodies merely because the OAuth scope was granted. |
| `manage` | `Calendars.ReadWrite.Shared` | `write`, `delegateWithoutPrivateEventAccess`, or `delegateWithPrivateEventAccess`. `write` and delegates can edit non-private events. Delegation applies to the owner's primary calendar; delegates are inside the same organization. |
| private-event read/manage | same OAuth scope as above | Requires `delegateWithPrivateEventAccess` (or Exchange `FullAccess`). Microsoft documents `object not found` behavior when the caller lacks the necessary private-item right. |

The `calendar` resource's `canEdit` and `canViewPrivateItems` values are useful
preflight signals, but Graph/Exchange remains authoritative. A `custom` sharing
role cannot be safely mapped to local `read` or `manage` without testing the
actual attempted operation.

### Operation matrix

| CR-0067 operation | Owner-primary route and scope | Result |
|---|---|---|
| Get configured calendar | `GET /users/{owner}/calendar`; `Calendars.Read.Shared` | Supported. |
| List events | `GET /users/{owner}/calendar/events`; `Calendars.Read.Shared` | Supported. This returns single instances and series masters, not expanded occurrences. |
| List a time range / expanded occurrences | `GET /users/{owner}/calendarView?startDateTime=...&endDateTime=...`; `Calendars.Read.Shared` | Supported. Use this contract for date-range listing. |
| Get event | `GET /users/{owner}/calendar/events/{owner-event-id}`; `Calendars.Read.Shared` | Supported with owner-view ID only. |
| Search events | No separate shared-search API exists. Implement as supported OData filtering over the appropriate owner-view `events` or `calendarView` collection; `Calendars.Read.Shared` | Supported only to the extent the existing search expression uses query options supported by that collection. It inherits the same route, role, private-item, recurrence, and ID rules. |
| Create appointment/event | Documented shared form: `POST /me/calendars/{recipient-calendar-id}/events`; `Calendars.ReadWrite.Shared` plus write/delegate role | Supported through an explicit recipient-view target. Direct owner-view `.Shared` write is not established by the dedicated guide. |
| Create meeting (event with attendees) | Same recipient-mounted create route | Graph sends invitations automatically, the calendar owner is the organizer/from identity, and the signed-in sharee/delegate is the message sender. **Exclude under the current CR unless an explicit human-confirmed on-behalf-of meeting-send contract is added.** Help and audit text alone are insufficient to make an externally communicating mutation silent or reversible. |
| Update or reschedule | Documented target model is the recipient-mounted event with `Calendars.ReadWrite.Shared` plus write/delegate role; generic PATCH path shapes exist for both views | Appointment-only updates are supported through the recipient-view target. **Exclude or human-confirm attendee-bearing updates** because meeting changes can send updates. An attendees-only patch normally notifies changed attendees; recurring-series updates can generate multiple messages. Online-meeting body updates must preserve the meeting blob. |
| Delete a non-meeting appointment | Recipient-mounted event; `Calendars.ReadWrite.Shared` plus write/delegate role | Supported; success is `204 No Content`. Direct owner-view `.Shared` delete is not established by the dedicated guide. |
| Delete a meeting from the organizer's calendar | Event DELETE | **Exclude from `shared delete_event` in this CR.** Microsoft states this sends a cancellation message to attendees, which crosses CR-0067's explicit boundary excluding meeting cancellation on behalf of an owner. The handler must fetch/inspect the event and reject shared deletion when it would cancel a meeting. A later confirmed `cancel_meeting` design can use the explicit cancel action and display its notification semantics. |
| Accept, tentative accept, decline, or explicit cancel on behalf of owner | Action routes on the event | Excluded as CR-0067 already states. Delegate response delivery and organizer-only cancellation require their own safety contract. |

There is no general `sendUpdates=false` control documented for create, update,
or delete event. Tool descriptions and confirmations must not imply these
operations are silent.

### Personal Microsoft accounts

The permissions reference explicitly marks delegated
`Calendars.Read.Shared` as available for personal Microsoft accounts. It does
**not** mark `Calendars.ReadWrite.Shared` as available for personal accounts,
and that permission's description is limited to calendars "in the
organization." Accordingly:

* personal-account shared calendar support must be read-only;
* `manage` must be rejected before authentication/Graph for a personal account;
* Microsoft's owner-mailbox walkthrough does not establish `/users/{owner}`
  direct routing for a personal-account shared calendar. The defensible MSA
  contract is recipient-view access to a calendar mounted under `/me/calendars`;
  because CR-0067 currently mandates owner routing, personal-account aliases
  must be excluded from the first owner-route implementation unless the design
  adds the recipient-view target kind described above.

This is deliberately stricter than inferring support from the generic event
API's personal-account row. Those method pages list `Calendars.Read` or
`Calendars.ReadWrite`; they do not override the account availability and
scenario semantics of the `.Shared` permissions.

### Required CR-0067 implementation corrections

1. Support direct owner routing only for reads of the configured owner's
   primary calendar; do not infer delegated `.Shared` writes from generic route
   shapes.
2. Add an explicit recipient-view target kind with a mounted calendar ID for
   documented writes and custom shared calendars.
3. Keep `list_calendars` as recipient-view discovery and filter/validate
   mounted results against the configured owner; do not enumerate owner
   calendars.
4. Keep target/mailbox-view metadata with every calendar and event ID; never
   translate or reuse IDs across views implicitly.
5. Enforce the actual sharing role in addition to the local OAuth profile, and
   describe limited/private visibility accurately.
6. Restrict personal accounts to read-only recipient-view calendars; otherwise
   defer personal shared access from the first owner-route release.
7. Block shared `delete_event` when deletion would cancel an organizer meeting.
8. Limit current shared create/update/reschedule/delete to appointments without
   attendees. Attendee-bearing mutations require a separate explicit
   human-confirmed on-behalf-of notification contract before inclusion.

## Sources

All sources are first-party Microsoft documentation:

* [Get shared or delegated Outlook calendar and its events](https://learn.microsoft.com/en-us/graph/outlook-get-shared-events-calendars) — owner-primary direct routes, recipient-view custom calendar routes, non-interchangeable IDs, and `.Shared` read/write semantics.
* [Create Outlook events in a shared or delegated calendar](https://learn.microsoft.com/en-us/graph/outlook-create-event-in-shared-delegated-calendar) — `Calendars.ReadWrite.Shared`, delegate/sharee meeting creation, owner organizer/from identity, sharee sender identity, and response delivery.
* [Share or delegate a calendar in Outlook](https://learn.microsoft.com/en-us/graph/outlook-share-or-delegate-calendar) — sharing/delegation setup, role meanings, delegate restrictions, `canEdit`, and private-event visibility.
* [Microsoft Graph permissions reference](https://learn.microsoft.com/en-us/graph/permissions-reference#calendarsreadshared) — identifiers, descriptions, and personal-account availability for `Calendars.Read.Shared` and `Calendars.ReadWrite.Shared`.
* [Get calendar](https://learn.microsoft.com/en-us/graph/api/calendar-get?view=graph-rest-1.0), [list events](https://learn.microsoft.com/en-us/graph/api/calendar-list-events?view=graph-rest-1.0), [list calendarView](https://learn.microsoft.com/en-us/graph/api/user-list-calendarview?view=graph-rest-1.0), and [get event](https://learn.microsoft.com/en-us/graph/api/event-get?view=graph-rest-1.0) — v1.0 read routes, recurrence behavior, query contract, and private-item prerequisites.
* [Create event](https://learn.microsoft.com/en-us/graph/api/calendar-post-events?view=graph-rest-1.0), [update event](https://learn.microsoft.com/en-us/graph/api/event-update?view=graph-rest-1.0), and [delete event](https://learn.microsoft.com/en-us/graph/api/event-delete?view=graph-rest-1.0) — v1.0 owner routes, invitation/update notifications, update caveats, cancellation-on-delete, and response codes.
* [Cancel event](https://learn.microsoft.com/en-us/graph/api/event-cancel?view=graph-rest-1.0) — organizer-only behavior and distinction between explicit cancellation and delete.
* [Obtain immutable identifiers for Outlook resources](https://learn.microsoft.com/en-us/graph/outlook-immutable-id) — case sensitivity, mailbox lifetime, event support, and calendar-container limitations.
* [calendarPermission resource](https://learn.microsoft.com/en-us/graph/api/resources/calendarpermission?view=graph-rest-1.0) — authoritative `calendarRoleType` values and their read/write/private-event meanings.
