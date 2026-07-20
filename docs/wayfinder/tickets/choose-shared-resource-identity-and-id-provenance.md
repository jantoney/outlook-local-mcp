# Choose shared resource identity and ID provenance

Labels: `wayfinder:grilling`
Status: closed
Assignee: /root
Blocked by: `Verify the shared calendar Graph contract`, `Verify the shared mail and send Graph contract`

## Question

What exact persisted identity must a shared resource alias carry, how should
primary versus custom calendars and mailboxes be represented, and how must
owner-route provenance accompany returned IDs so an ID can never be replayed
silently against the wrong owner or the signed-in account?

## Resolution

Use two distinct identity layers: durable human-facing workflow selectors and
immutable internal provenance identities.

### Persisted account and alias identity

Every persisted signed-in account has an immutable internal account ID in
addition to its unique human-facing label and UPN. Legacy records receive and
persist an internal ID during migration. Logout, login, token refresh, profile
changes, and server restart preserve that ID. Removing and re-adding an account
creates a new internal ID even when the label and UPN are reused.

Every shared resource record has an immutable internal resource ID and exactly
one of these kinds:

| Kind | Persisted routing identity | Supported view |
|---|---|---|
| `mailbox` | resource-owner UPN or Graph-compatible user locator | owner mailbox |
| `owner_primary_calendar` | resource-owner UPN or Graph-compatible user locator | owner primary-calendar view, read-only |
| `mounted_calendar` | resource-owner identity plus the human-selected mounted calendar ID | signed-in account's mounted-calendar view |

One alias names one resource kind and one mailbox view. Mail and calendar
resources use separate aliases even when they share an owner. The display
alias can be renamed while preserving its internal ID. Owner, kind, and view
are immutable; retargeting requires delete-and-recreate, which creates a new
resource ID.

Creating a mounted-calendar alias performs `/me/calendars` discovery for the
configured owner and requires the human to select the intended calendar. It
persists the exact mounted calendar ID. A missing or invalid mounted ID causes
an explicit re-selection flow and is never silently rebound.

### Durable workflow selection

Codex workflows persist human-facing selectors such as `account="work"` and
`shared_resource="finance-mail"`. These names resolve to the current internal
identities on every run. Therefore a workflow that checks or sends through a
named account can resume after that account and alias are deliberately
recreated with the same names.

Workflow selectors do not carry item provenance. Old references to particular
messages, drafts, events, folders, conversations, or attachments remain
invalid after account or alias removal. This preserves `account.remove` and
shared-resource removal as explicit local revocation boundaries while keeping
name-based automations durable.

### Signed item provenance

Every item returned from a shared resource has a versioned, self-contained,
HMAC-signed `resource_ref`. Its payload binds:

* immutable signed-in account ID;
* immutable shared resource ID;
* resource and mailbox-view kind;
* item kind; and
* the Graph ID chain needed for that item, including parent message ID for an
  attachment where applicable.

The signature uses a locally persisted signing key. References survive server
restarts and have no fixed TTL. Each use verifies the signature and re-resolves
the current account, shared resource, capability profile, allowlist, and Graph
authorization. Account removal, shared-resource removal, signing-key rotation,
or Graph resource deletion invalidates a reference. A reference grants no
Graph authority by itself.

Shared follow-up operations require `resource_ref`; they do not accept a raw
Graph ID plus an alias. Own-resource operations retain raw Graph IDs for
backward compatibility.

Text and summary output display `resource_ref` directly. Raw output preserves
the complete unmodified Graph JSON and returns a separate provenance sidecar
mapping each relevant Graph ID to its signed reference. Raw Graph IDs are
informational and cannot be used as shared follow-up parameters.

### Decisions captured during grilling

- One shared resource alias identifies exactly one resource kind and Graph
  mailbox view. A resource owner exposing both mail and calendar resources uses
  separate aliases, such as `finance-mail` and `finance-calendar`.
- Every calendar alias explicitly declares either `owner_primary_calendar` or
  `mounted_calendar`. The server never infers or silently switches the view.
- Creating a `mounted_calendar` alias discovers the signed-in account's mounted
  calendars for the configured owner and requires human selection. The exact
  mounted calendar identity is persisted. If it becomes invalid, the server
  requires re-selection and never silently rebinds the alias.
- Shared results expose an opaque `resource_ref` that binds the signed-in
  account, alias identity, resource kind, mailbox view, and Graph item ID.
  Follow-up shared operations require that reference and do not accept a raw
  Graph ID paired with an alias. Own-resource raw IDs remain backward
  compatible.
- A shared resource record has an immutable internal identity. Its display
  alias can be renamed without invalidating references, but its owner, resource
  kind, and mailbox view cannot be edited. Retargeting requires removal and
  recreation with a new identity, invalidating every old `resource_ref`.
- `resource_ref` is a versioned, self-contained, HMAC-signed value backed by a
  locally persisted signing key. It survives restarts without a server-side ID
  registry and detects changes to account, shared resource identity, view,
  kind, and Graph identifiers. It grants no Graph authority. Signing-key
  rotation explicitly invalidates all outstanding references.
- Every persisted signed-in account has an immutable internal account ID.
  Labels and UPNs remain human-facing. Removing and re-adding the same label or
  UPN creates a new account identity, so references issued for the prior
  account instance fail validation.
- Text and summary output expose `resource_ref` directly for shared items. Raw
  output preserves the complete unmodified Graph JSON and returns a separate
  provenance sidecar mapping Graph IDs to signed references. Shared follow-up
  operations accept only `resource_ref`; raw Graph IDs are informational.
- Signed references have no fixed time-to-live. Each use revalidates the
  account, shared resource identity, capability, allowlist, and Graph access;
  references instead become invalid through explicit identity removal, alias
  removal, signing-key rotation, or Graph resource deletion.
- Durable workflows use account labels and shared resource aliases as selectors
  resolved on each run. Recreating the same names lets the workflow continue,
  while item references from the removed identities remain invalid.
