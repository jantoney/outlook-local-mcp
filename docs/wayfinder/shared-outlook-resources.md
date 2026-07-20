# Shared Outlook resources

Labels: `wayfinder:map`

## Destination

An implementation-ready technical route for approved CR-0067 and CR-0068 in which every
Graph endpoint constraint, resource identity rule, capability boundary, SDK
seam, and rollout dependency is decided before coding begins.

## Notes

- Source branch: `plan/graph-attachments-personal-auth` at `d9e2306`.
- Governing specifications: [CR-0067](../cr/CR-0067-per-account-shared-outlook-resources.md) and [CR-0068](../cr/CR-0068-granular-mail-action-policies.md), approved 2026-07-19.
- Follow `AGENTS.md`, the domain glossary in `CONTEXT.md`, and the repository governance process.
- Wayfinder is planning-only. It resolves decisions; implementation begins through the implementation workflow after the map is clear.
- Microsoft Graph documentation and the installed `microsoftgraph/msgraph-sdk-go` v1.96.0 source are authoritative.
- DeepWiki is registered in `.deepwiki` but was not callable when this map was charted.
- Canonical terms are signed-in account, resource owner, shared resource, shared resource alias, shared access profile, and resource-level right.

## Decisions so far

- [Verify the shared calendar Graph contract](tickets/verify-shared-calendar-graph-contract.md) — direct owner routes cover primary-calendar reads, while shared writes and custom calendars use recipient-mounted routes; IDs are view-specific and personal accounts have documented shared-read support only.
- [Verify the shared mail and send Graph contract](tickets/verify-shared-mail-and-send-graph-contract.md) — shared mailbox operations use owner routes; shared send additionally needs Exchange rights, is accepted asynchronously without idempotency or conditional-send support, and defaults its sent copy to the owner's Sent Items.
- [Inventory the shared target SDK seams](tickets/inventory-shared-target-sdk-seams.md) — the 42 executable `Me()` calls span 26 files, but `Me()` and `Users().ByUserId(owner)` share the same SDK user-builder type, enabling one narrow target-to-user seam while leaving handler-specific builders intact.
- [Choose shared resource identity and ID provenance](tickets/choose-shared-resource-identity-and-id-provenance.md) — workflows resolve stable account and alias names, while each alias has one immutable resource/view identity and shared items use restart-stable signed references that cannot cross accounts, aliases, or mailbox views.
- [Verify Microsoft account-type detection](tickets/verify-microsoft-account-type-detection.md) — the validated token tenant identifies a personal consumer context or organizational context; Graph `/me` cannot establish home-account type, and missing or invalid tenant data remains unknown.
- [Verify mail move, delete, and archive Graph contract](tickets/verify-mail-move-delete-archive-graph-contract.md) — move, delete, and permanent delete have distinguishable Graph requests, while ordinary move, Archive, trash, and restore require destination-aware policy gates around the shared move action.
- [CR-0068](../cr/CR-0068-granular-mail-action-policies.md) — approved independent per-target mail capabilities supersede cumulative manage tiers and add separately gated move, Archive, trash, restore, and permanent-delete operations.
- [Choose shared capability and account compatibility rules](tickets/choose-shared-capability-and-account-compatibility.md) — calendar aliases use target-local read/manage profiles, mail targets use independent action switches, token-tenant compatibility fails closed, and reauthentication occurs only when the account's effective scope union changes.
- [Choose the own and shared Graph routing seam](tickets/choose-own-and-shared-graph-routing-seam.md) — authorized immutable targets select one fail-closed typed SDK user root by mailbox view, while target policy, signed-reference checks, handler behavior, and real-SDK route tests remain behind separate narrow modules.
- [Choose the shared send safety contract](tickets/choose-shared-send-safety-contract.md) — shared send binds a single-use human confirmation to canonical target, sender, recipient, attachment, and version evidence, revalidates immediately before one non-retried POST, and represents ambiguous results as uncertain.
- [Choose the shared resource rollout and verification plan](tickets/choose-shared-resource-rollout-and-verification-plan.md) — persistence and fail-closed routing land before handlers; public contracts travel with each surface change; representative live evidence gates advertised support, while tenant-specific gaps narrow it without weakening authorization.

## Not yet specified

None. Live tenant evidence may narrow the advertised compatibility matrix, but
the governing behavior, safety gates, migration, and release response to that
evidence are specified.

## Out of scope

- Application permissions, daemon access, or tenant-wide mailbox enumeration.
- Creating Outlook sharing invitations or Exchange delegation and shared-send assignments.
- Cross-tenant shared mailboxes, public folders, archive mailboxes, subscriptions, and change notifications.
- Shared contacts, tasks, OneDrive, SharePoint, and Teams resources.
- Free-form resource-owner parameters on calendar or mail verbs.
