# Choose shared capability and account compatibility rules

Labels: `wayfinder:grilling`
Status: closed
Assignee: /root
Blocked by: none

## Question

How should shared calendar and shared mail profiles compose with the existing
mail profile, which invalid combinations must configuration reject, and what
behavior must distinguish work/school accounts from personal Microsoft
accounts without making unreliable identity-type assumptions?

## Working decisions

- Own-mail policy and every shared-mail alias policy are independent logical
  capability boundaries. An account's OAuth scopes are the union required by
  its configured targets, but the MCP checks the selected target before it
  constructs or calls a `/me` or `/users/{owner}` route.
- Aggregate mail operations remain statically discoverable because one target
  may allow an operation another target denies. Help and status expose the
  target policy matrix; denied target routes are never constructed.
- This is an MCP enforcement boundary, not token isolation. A token carrying a
  union scope retains that Microsoft-granted authority if stolen or used
  outside this MCP.
- Classify the current token tenant context as `personal` only when its
  validated tenant ID is Microsoft's fixed consumer tenant ID,
  `organizational` for another validated tenant GUID, and `unknown` when the
  tenant ID is unavailable or invalid. Do not infer account origin from email,
  tenant names, Graph `/me`, or opaque account-ID formatting.
- Personal and unknown token contexts fail closed for capabilities documented
  only for work/school tenants: shared mail read, manage, and send, plus shared
  calendar management. Organizational guest contexts are treated as
  organizational for endpoint compatibility; Microsoft remains authoritative
  for the resource-level rights actually granted.
- Every shared-calendar alias has its own `off`, `read`, or `manage` profile.
  `read` is compatible with personal and organizational token contexts.
  `manage` is exposed only for mounted-calendar aliases in an organizational
  token context; owner-primary aliases remain read-only because their direct
  owner route is not the supported shared-write view. Unknown contexts fail
  closed to `read` at most.
- Shared-mail draft and send authority are distinct logical boundaries. Draft
  authority may expose draft creation and editing without registering a route
  that can deliver the message. Send authority is separately configurable for
  each own-mail target and each shared-mail alias; it is never inferred merely
  because draft operations are allowed.
- Shared-mail capabilities are independent switches rather than a cumulative
  `manage` tier. In particular, ordinary move, Archive, deletion, permanent
  deletion, draft, and send can be allowed or denied separately for every
  own-mail target and shared-mail alias.
- Graph exposes move, `DELETE`, and permanent-delete request shapes, but
  ordinary move, Archive, moving to Deleted Items, and restoring from Deleted
  Items all share the `/move` action with different destinations. The MCP must
  resolve and classify the destination before constructing that request, and
  an ordinary-move gate must reject Archive and deletion-class destinations.
  These distinctions are MCP policy boundaries only because all mutations use
  the same `Mail.ReadWrite` OAuth scope.
- Restore is its own per-target capability rather than being implied by
  ordinary move. Its MCP route may use Graph `/move` only after verifying the
  message is in Deleted Items and the destination is neither Archive nor a
  deletion-class folder.
- Approved CR-0068 expands delivery scope with five mail verbs: ordinary move,
  Archive, trash, restore, and permanent delete. Their gates are independent
  for own mail and every shared-mail alias; permanent delete defaults off.
- Policy changes take effect in local target guards immediately. Disconnect and
  clear authentication material only when the account's deduplicated OAuth
  scope union changes; a same-scope policy change does not interrupt the
  account. A broader token retained because another target still needs its
  scope does not authorize a locally disabled action.

## Resolution

Use target-local capability policies rather than cumulative account-wide mail
tiers. The signed-in account's own mailbox and each shared-mail alias carry
independent mail action policies. The MCP resolves the selected target and
checks its exact capability before constructing either an own-resource or
owner-resource Graph route. Aggregate operations remain statically
discoverable, while help and status expose the target policy matrix.

Mail actions are independent switches: read, draft, ordinary move, Archive,
trash, restore, permanent delete, and send. Draft does not imply send; ordinary
move cannot target Archive or deletion-class folders; and restore is not
implied by move. CR-0068 adds the five received-message mutation verbs and
defaults permanent deletion off. All switches are local MCP enforcement over
the broader OAuth permissions required by Microsoft Graph.

Every shared-calendar alias has an independent `off`, `read`, or `manage`
profile. Shared read is compatible with personal and organizational token
contexts. Shared management is restricted to recipient-mounted calendars in an
organizational token context. Direct owner-primary aliases remain read-only.

Classify token tenant context from the validated tenant ID: the fixed Microsoft
consumer tenant is personal, another validated GUID is organizational, and
missing or invalid data is unknown. Do not infer account origin from profile
fields or email shapes. Personal and unknown contexts fail closed for
work/school-only shared mail and shared-calendar management. Organizational
guest contexts may attempt organizational endpoints, but Microsoft resource
rights remain authoritative.

The account requests the deduplicated OAuth-scope union required by all of its
configured targets. A policy change applies locally immediately. Reauthenticate
only when that union changes; otherwise retain the session. A broader token
retained for another target never bypasses the selected target's local policy.
