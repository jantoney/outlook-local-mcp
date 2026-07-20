# Outlook Local MCP

This context defines the identity and resource language used when the MCP acts
for Microsoft accounts across own, shared, and delegated Outlook data.

## Language

**Signed-in account**:
The persisted Microsoft identity whose delegated token authorizes a Graph request and whose account instance has an immutable internal identity distinct from its label and UPN.
_Avoid_: Shared account, login account, mailbox account

**Resource owner**:
The Microsoft identity or mailbox that owns the calendar or mail resource being targeted; it may differ from the signed-in account.
_Avoid_: Target account, shared account

**Own resource**:
A calendar or mailbox resource owned by the signed-in account and addressed through the Graph `me` identity.
_Avoid_: Personal resource, default resource

**Shared resource**:
A calendar or mailbox resource owned by a resource owner and made accessible to a signed-in account through Outlook sharing or Exchange delegation.
_Avoid_: Shared account, foreign resource

**Shared resource alias**:
An account-scoped local name that identifies exactly one allowlisted resource kind, Graph mailbox view, and resource owner. Mailboxes and calendars use separate aliases even when they have the same owner.
_Avoid_: Owner parameter, mailbox address

**Shared resource identity**:
The immutable identity behind a shared resource alias; renaming preserves it, while changing owner, kind, or mailbox view requires a new identity.
_Avoid_: Alias name, resource address

**Shared resource reference**:
An opaque value that binds an item identifier to its signed-in account, shared resource identity, resource kind, and mailbox view.
_Avoid_: Shared ID, Graph ID

**Workflow selector**:
A durable account label and optional shared resource alias that a workflow resolves to the current configured identities each time it runs.
_Avoid_: Resource reference, internal account ID

**Resolved resource target**:
The immutable per-request selection of one signed-in account and exactly one own or shared resource identity after workflow selectors have been resolved and authorized.
_Avoid_: Raw alias, caller-supplied owner

**Mailbox view**:
The Graph identity space in which a resource and its identifiers are valid: the signed-in recipient view or the resource-owner view. Ownership alone does not select the view; mounted calendars remain in the recipient view.
_Avoid_: Own-versus-shared route

**Owner-primary calendar**:
A resource owner's primary calendar addressed in the owner's mailbox view and used only with identifiers from that view.
_Avoid_: Direct calendar, owner calendar

**Mounted calendar**:
A shared primary or custom calendar represented in the signed-in account's mailbox view and used only with identifiers from that mounted view.
_Avoid_: Local copy, shared calendar

**Mail profile (legacy)**:
The cumulative own-mail capability value accepted only for backward-compatible migration into a mail action policy.
_Avoid_: New policy model, shared access policy

**Mail action policy**:
The independent set of read, draft, move, Archive, trash, restore, permanent-delete, and send capabilities allowed for exactly one own mailbox or shared-mail alias.
_Avoid_: Manage tier, cumulative mail profile

**Shared access profile**:
The capability level accepted for a signed-in account when it acts on shared resources.
_Avoid_: Shared permission, delegation role

**Resource-level right**:
An Outlook or Exchange sharing, delegation, Send As, or Send on Behalf right granted by a resource owner or administrator.
_Avoid_: OAuth permission, app consent

**Token tenant context**:
The tenant in which Microsoft issued the current delegated token, classified as personal consumer, organizational, or unknown from the validated tenant ID. It governs endpoint compatibility but does not prove a guest user's home-account type.
_Avoid_: Account origin, email account type

**Shared send review snapshot**:
The canonical, single-use human-review evidence for one shared-mailbox draft version, target identity, sender presentation, recipient sets, and attachment set.
_Avoid_: Confirmation boolean, send token

**Uncertain send outcome**:
A send attempt whose dispatch may have reached Exchange but whose acceptance was not observed, so it can be neither retried automatically nor described as failed or unsent.
_Avoid_: Send failed, not sent

**Live compatibility gate**:
A representative tenant-backed check required before one Outlook resource scenario is included in the advertised support matrix; missing evidence narrows that matrix rather than weakening authorization.
_Avoid_: Best-effort test, universal tenant proof

**Uncertain destructive outcome**:
A permanent-delete attempt whose dispatch may have reached Exchange but whose result was not observed, so it cannot be retried automatically or described as definitely deleted or retained.
_Avoid_: Delete failed, message retained
