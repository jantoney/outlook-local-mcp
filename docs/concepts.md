# Concepts

Core concepts for outlook-local-mcp. Use these as background when the Quick Start or Troubleshooting guide is not enough.

## Output tiers

All read tools accept an `output` parameter with three modes. Write tools return concise text confirmations unconditionally.

**`text`** (default) returns pre-formatted plain text optimised for LLM context consumption. Collections render as numbered lists with human-readable fields and a total count. A 12,000-token raw Graph API event becomes roughly 150-800 tokens in text mode — a 60-70% reduction. The LLM can pass this through without additional formatting.

**`summary`** returns compact JSON with a deliberately curated field set per tool. Useful when the LLM needs structured data for programmatic reasoning. Nested objects are flattened: start/end become plain dateTime strings, organizer becomes a name string. Summary mode includes a `displayTime` field with a pre-formatted human-readable time string.

**`raw`** returns the full, unmodified Graph API serialisation including empty values. Use this when you need HTML body content, recurrence patterns, attendee email addresses, or other detailed fields. `raw` is never the default; it must be requested explicitly.

Invalid values return an error: `output must be 'summary', 'raw', or 'text'`.

## Multi-account model and UPN identity

The server supports managing multiple Microsoft accounts simultaneously. Each account is identified by a human-chosen **label** (e.g., `work`, `personal`). The User Principal Name (UPN, e.g. `alice@contoso.com`) resolved from Microsoft Graph `/me` is the **canonical identity** and is persisted to `accounts.json` in the `upn` field.

UPN is available immediately at startup without a Graph API call and is shown in all account surfaces (`account.list`, `system.status`, elicitation prompts, write-tool confirmations). Any tool that accepts an `account` parameter resolves it by label first, then by case-insensitive UPN fallback — so `account=alice@contoso.com` and `account=work` both target the same entry.

Account selection logic for calendar and mail verbs:

- **Explicit selection** — pass `account: "label"` or `account: "upn"`.
- **Single authenticated account, no others** — auto-selected silently.
- **Single authenticated account with disconnected siblings** — auto-selected, with an advisory naming the disconnected accounts by UPN so the LLM can surface them.
- **Multiple authenticated accounts** — the server uses MCP Elicitation to prompt for selection.
- **All accounts disconnected** — error lists disconnected accounts by UPN and suggests `account.login`.
- **No accounts registered** — error directs to `account.add`.
- **Elicitation unsupported or fails** — the default account is used as a fallback; if no default exists, the error lists available accounts and suggests using the `account` parameter.

Each account has its own token cache partition, auth record file, and Graph client instance. The `accounts.json` file stores only non-secret identity metadata. Tokens and credentials are managed separately by the OS-native token cache. The file is written atomically to prevent corruption on sudden exit.

## Auto-default account semantics

A **default account** is registered automatically at startup using the server's configured credentials (`CLIENT_ID`, `TENANT_ID`, `AUTH_METHOD`). Additional accounts added via `account.add` are persisted to `accounts.json` and restored on subsequent startups with silent token acquisition from the per-account cache (see CR-0064).

At startup the server performs a silent token probe (5-second timeout) to pre-authenticate persisted accounts. Accounts with expired tokens are registered as **disconnected** — they remain visible in `account.list` and `system.status` and can be reconnected explicitly via `account.login`, or will be re-authenticated automatically by the auth middleware on the first tool call that targets them.

The default account cannot be removed via `account.remove`.

## MCP elicitation requirement

Multi-account features (account selection prompts, inline authentication during `account.add`) use the MCP Elicitation API. The server declares the `elicitation` capability at startup. MCP clients that support elicitation receive interactive prompts; clients that do not fall back to the default account for account selection and receive authentication feedback as tool result text.

For `device_code` auth without elicitation, `account.add` uses a two-call pattern: the first call returns the device code and keeps the authentication goroutine alive in the background; the second call with the same label picks up the completed authentication and registers the account.

## Read-only mode

Set `OUTLOOK_MCP_READ_ONLY=true` to disable all write operations. All write verbs (`calendar.create_event`, `calendar.create_meeting`, `calendar.update_event`, `calendar.update_meeting`, `calendar.delete_event`, `calendar.cancel_meeting`, `calendar.respond_event`, `calendar.reschedule_event`, `calendar.reschedule_meeting`, `mail.create_draft`, `mail.create_reply_draft`, `mail.create_forward_draft`, `mail.update_draft`, `mail.delete_draft`) return an error when invoked. Read and search verbs remain fully functional.

```bash
OUTLOOK_MCP_READ_ONLY=true ./outlook-local-mcp
```

## Independent mail action policies

The mail schema is stable at startup. Each account's own mailbox has independent `read`, `draft`, `move`, `archive`, `trash`, `restore`, `permanent_delete`, and `send` switches. The server checks the exact switch before constructing a Graph route. A broad OAuth token is never treated as local authorization for a disabled action.

`account.list` and `system.status` show the effective matrix. Use `account.set_mail_policy` with an account label and one or more boolean switches; omitted switches remain unchanged. New explicitly added accounts start with every switch off unless the legacy `mail_profile` input is supplied. Permanent deletion and the newly introduced filing actions never become enabled through legacy migration.

Legacy values remain accepted temporarily and map without granting new behavior: `calendar_only` enables nothing; `mail_read` enables only read; `mail_manage` enables read and draft; and `mail_send` enables read, draft, and send. Global `MAIL_ENABLED`, `MAIL_MANAGE_ENABLED`, and `MAIL_SEND_ENABLED` remain migration/default inputs for the implicit account and legacy records.

OAuth scopes are derived from the enabled actions. Read alone contributes `Mail.Read`; draft or any filing/deletion action contributes `Mail.ReadWrite`; send contributes `Mail.ReadWrite` and `Mail.Send`. A policy change applies locally on the next request. The account disconnects and clears local authentication only when the required scope set changes; same-scope policy edits keep the session connected. Microsoft consent is not revoked automatically.

## Token tenant context and OAuth scope union

Account diagnostics classify only the directory context that issued a validated token. The fixed Microsoft consumer tenant GUID `9188040d-6c67-4c5b-b112-36a304b66dad` is reported as `personal`; another validated tenant GUID is `organizational`; missing, malformed, or unavailable evidence is `unknown`. An organizational token context does not prove that the user's home identity is organizational because personal Microsoft accounts can be guests in an organization.

The server never infers this context from an email address, UPN suffix, tenant display name, Graph profile, authority alias such as `common`, or an opaque account identifier. `unknown` remains explicit when validated token tenant evidence is unavailable.

Each account has one deterministic, deduplicated OAuth scope union calculated from all configured policies. Policy edits take effect locally immediately. Authentication is cleared only when the effective union changes; an edit that produces the same union keeps the account connected. This minimizes consent while avoiding unnecessary sign-in prompts.

`account.list` and `system.status` label `oauth_scopes` separately from `outlook_resource_rights`. OAuth scopes describe delegated consent requested from Microsoft identity. Outlook resource rights are the server's local action policy. Neither proves that Exchange grants access to a particular shared mailbox, folder, or calendar; Graph remains authoritative for resource-level access.

## Shared calendar aliases

Shared calendars are explicit account-scoped allowlist entries, not additional signed-in accounts. Calendar and mail aliases are separate families even when they name the same owner. Every calendar alias has an immutable resource ID, one owner, one kind, one mailbox view, and an exact `off`, `read`, or `manage` profile.

An `owner_primary_calendar` identifies the owner's primary calendar in owner view. It is available only with an organizational token context and remains read-only. A `mounted_calendar` identifies one calendar in the signed-in recipient's view. Creation and reselection perform fresh `/me/calendars` discovery filtered by the configured owner and require the human to confirm the exact mounted calendar ID. Mounted read supports validated personal and organizational contexts; unknown tenant context fails closed. Mounted manage additionally requires an organizational context and an editable discovery result.

Use `account.discover_calendar_aliases`, then `account.add_calendar_alias`. Listing shows immutable identity and exact policy. Renaming changes only the human selector. Reselection changes only the mounted ID after fresh confirmation. Owner, kind, and mailbox view cannot be edited; retargeting requires idempotent removal and recreation, which creates a new resource ID. A missing mount is never silently rebound.

Shared calendar read contributes `Calendars.Read.Shared`; manage contributes `Calendars.ReadWrite.Shared`. These join the account-wide OAuth scope union but do not replace target-local authorization or Exchange sharing rights.

`calendar.list_events` and `calendar.search_events` accept `shared_resource`. Owner-primary aliases route only through `/users/{owner}/calendarView`; mounted aliases stay in the signed-in recipient view and route only through `/me/calendars/{mounted-id}/calendarView`. Owner text is provenance for discovery and is never substituted into a mounted read route. Their shared text and summary results expose a signed `resource_ref` for each event. A shared `calendar.get_event` must receive that reference with the same alias; it rejects raw `event_id` addressing. References bind the account, immutable resource identity, resource kind, mailbox view, item kind, and Graph ID, and are revalidated against current configuration on every use. Removing and recreating an alias invalidates its old references even when the human alias is reused.

Raw shared-event output keeps the existing Graph-derived event object or array under `data` and returns signed references separately under `provenance`. This sidecar preserves the raw event shape instead of injecting local fields. Omitting `shared_resource` preserves existing own-calendar `/me` routes, raw `event_id` follow-ups, and response shapes.

An organizational mounted alias with profile `manage` can create, update, reschedule, and delete non-meeting events through its exact `/me/calendars/{mounted-id}/events` route. Here, non-meeting means Graph explicitly reports both zero attendees and `isOnlineMeeting=false`; missing classification fails closed. Mounted create and update cannot enable online-meeting capability. Follow-up writes require `shared_resource` and the target-bound `resource_ref` from a mounted read; raw shared event IDs are rejected. Before update, reschedule, or delete, the server reads the event to enforce that classification and rechecks the current account, alias, profile, reference, client, and route before starting the mutation. A policy change or alias removal between stages stops the unstarted write. Shared meeting actions, including `respond_event` and `cancel_meeting`, are outside this boundary and remain own-calendar only.

## Shared mail aliases

A shared-mail alias is a separate account-scoped allowlist entry for one owner-view mailbox routed through `/users/{owner}/...`. It has an immutable resource ID, owner, `mailbox` kind, and `owner` mailbox view. Renaming changes only the human selector and preserves identity. Retargeting requires idempotent removal and recreation, which creates a new identity and invalidates old target-bound references. A calendar and mail alias may use the same selector and owner without becoming the same resource.

New aliases start with all eight actions disabled: `read`, `draft`, `move`, `archive`, `trash`, `restore`, `permanent_delete`, and `send`. Use `account.set_mail_alias_policy` to change only explicitly supplied switches. Each alias policy is independent of the signed-in account's own-mail policy and every other alias. Personal and unknown token contexts reject shared-mail configuration and use locally; only validated organizational contexts are compatible. Exchange mailbox, folder, and Send As or Send on Behalf delegation remains authoritative.

Shared read contributes `Mail.Read.Shared`. Draft, filing, recovery, deletion, or send contributes `Mail.ReadWrite.Shared`; send also contributes `Mail.Send.Shared`. The server disconnects the account only when changing an alias alters the complete account-wide scope union. OAuth consent never authorizes an action disabled by the selected alias policy.

With `read` enabled, folder, message, search, conversation, and attachment read verbs accept `shared_resource` and route only through `/users/{owner}/...`. Shared folder and message text/summary results include signed target-bound references. To scope shared `list_messages` or `search_messages`, pass the `folder_ref` returned by `list_folders`; raw `folder_id` plus alias is rejected. Shared `get_message` requires the `resource_ref` returned by a message collection. Conversation and attachment metadata reads require a target-bound `message_ref`; attachment metadata returns `attachment_ref`, and download requires both the matching parent message and attachment references. Raw IDs plus alias are rejected. Raw output keeps Graph-derived data under `data` and returns local references in a separate `provenance` sidecar. References bind the account, immutable mailbox identity, owner view, item kind, and required Graph ID chain, and are revalidated against current policy on every use. Content previews remain the default for message collections and conversations; complete bodies require `output=raw`. Omitting `shared_resource` preserves existing own-mail `/me` behavior and output shapes.

With `draft` enabled, `create_draft`, `create_reply_draft`, `create_forward_draft`, `update_draft`, and `delete_draft` accept `shared_resource` and stay on that alias's exact `/users/{owner}/messages` route. Shared reply and forward creation require a target-bound `message_ref`; shared update and delete require the returned `draft_ref`. Raw message or draft IDs plus an alias are rejected. Update and delete first verify `isDraft=true`, then recheck the current account, alias policy, reference, client, and route before starting the mutation. Revocation between those stages stops the unstarted write.

Draft creation returns a `draft_ref` bound to the shared mailbox and current Graph draft ID. Reply and forward creation plus their required provenance PATCH form one bounded operation: after Graph creates the draft, the completion PATCH retains the already authorized immutable owner route instead of re-resolving an alias. If completion or reference emission cannot be confirmed, the result says `PARTIAL SUCCESS`, includes every available recovery identifier, and warns not to repeat the non-idempotent create operation. All later work with that draft requires fresh authority.

## Local draft attachments

`mail.add_attachment` attaches exactly one local file to an existing draft and requires the exact `draft` capability. `OUTLOOK_MCP_ATTACHMENT_ROOTS` is a platform path-list allowlist; an empty value disables local upload. Canonical paths outside those roots, including traversal and link escapes, are rejected. Own-mail files below 3 MiB use direct upload, files through 150 MiB use sequential resumable upload, and larger files are rejected.

Shared-mail attachment upload requires `shared_resource` plus the target-bound `draft_ref` and is intentionally limited to allowlisted files below 3 MiB. After the draft preflight and local file checks, the server reauthorizes the account, alias, draft policy, reference, client, and exact owner route immediately before the single direct POST. Verification retains that immutable route. Confirmations expose verified name, size, MIME type, attachment ID, shared target, and renewed draft provenance, but never the canonical local path or file content. If Graph accepted the attachment but verification or provenance is incomplete, the result reports `PARTIAL SUCCESS` and warns against repeating the upload until the draft is inspected. Large shared upload sessions are not advertised or started.

The 2026-07-21 compatibility gate had no representative live organizational shared mailbox and disposable large attachment, so it produced no positive route or completion evidence. The support decision is therefore explicit: shared attachments at or above 3 MiB are unsupported and remain locally rejected. Small shared attachments do not imply upload-session compatibility.

## Mail filing and recovery

`mail.move_message` files one referenced message into one referenced ordinary folder and requires only the selected target's `move` action. The new verb uses the same secure contract for own and shared mail: `message_ref` and `destination_folder_ref` must bind the same account, immutable mailbox, kind, and mailbox view. Existing own read defaults and raw-ID follow-ups remain unchanged; call `list_messages` or `search_messages` with `include_refs=true` to obtain an own source reference.

Call `list_folders` with `include_refs=true` to resolve the mailbox's well-known folders and receive signed `ordinary` or `reserved` destination classification. Archive, Deleted Items, Drafts, Outbox, recoverable-items folders, and every reserved or unclassified destination are rejected before the move route is constructed; those semantic actions have separate policy gates. The destination folder is resolved again, current authority is rechecked, and Graph's move action is attempted once on the immutable route. Success returns only the destination message's current ID and new signed reference. A timeout, connection loss, or 5xx is `uncertain` and must be reconciled by inspecting both folders before retrying.

`mail.archive_message` uses only Outlook's well-known One-Click Archive destination; it does not access the separate Exchange Online Archive Mailbox. `mail.trash_message` uses only Deleted Items and is reversible subject to mailbox policy. Each requires its independent target action and returns Graph's replacement message reference.

`mail.restore_message` first verifies that the referenced message is currently in Deleted Items, then accepts only a same-target destination classified as ordinary. It does not restore from Recoverable Items. `mail.permanent_delete_message` is a distinct destructive Graph action: even when explicitly enabled, every single message requires fresh MCP human confirmation bound to the unchanged message and exact target. Outlook clients cannot recover it, although retention or legal hold may still apply. Permanent deletion is never retried after an ambiguous response.

## Confirmed draft send

`mail.send_draft` requires the exact `send` capability and accepts only an existing draft. Draft capability does not imply send. Own mail retains its existing draft-ID flow. Shared mail requires `shared_resource` plus a target-bound `draft_ref`; raw IDs cannot reach the owner route.

Before shared elicitation, the server binds the immutable target, masked delegate and owner presentation, canonical From, nonempty change key, subject, separate normalized To/Cc/Bcc multisets, and every attachment page with identity and metadata. Missing evidence makes send unavailable. The body is not copied into elicitation and must be reviewed in Outlook. After acceptance, authority and the exact snapshot are checked again before one non-retried owner-route POST. Graph acceptance means only accepted for Exchange processing. Exchange chooses Send As or Send on Behalf from configured rights, and the owner's Sent Items is the documented default rather than an exclusive guarantee. Ambiguous outcomes require inspecting the owner's Drafts and Sent Items before any newly reviewed attempt.

## Headless and non-interactive authentication

Authentication is lazy — deferred until the first tool call rather than blocking at startup. Three flows are available, controlled by `OUTLOOK_MCP_AUTH_METHOD`:

**`device_code`** (default for well-known client IDs) — the server obtains a device code from Entra ID and delivers it to the user. If MCP Elicitation is supported, the user sees the code in a prompt; otherwise it appears as tool result text. The tool returns immediately; calling any tool after the user completes sign-in in their browser picks up the cached token automatically. Works in all environments including headless and Docker.

**`browser`** (default for custom app registrations) — the system browser opens to the Microsoft login page and the server listens on a localhost port for the OAuth callback. Requires an app registration with `http://localhost` redirect URI.

**`auth_code`** — the system browser opens for OAuth login. The user pastes the redirect URL back via MCP Elicitation or the `system.complete_auth` verb. Uses PKCE for security. Suitable for headless or remote environments where a localhost port cannot be opened.

On subsequent runs the server acquires tokens silently using the cached refresh token. No browser interaction is needed unless the refresh token expires (typically after 90 days of inactivity) or the token cache is cleared. When a token expires mid-session, the auth middleware detects the failure and re-initiates the configured flow with client-visible prompts.

## OAuth scopes used per feature

The server requests scopes incrementally. Expanding mail access after initial consent triggers a re-consent prompt.

| Feature | OAuth scope |
|---|---|
| Calendar (always active) | `Calendars.ReadWrite` |
| Account identity (always active) | `User.Read` |
| Own-mail `read` only | `Mail.Read` |
| Any draft, filing, recovery, or deletion action | `Mail.ReadWrite` |
| Own-mail `send` | `Mail.ReadWrite`, `Mail.Send` |
| Shared calendar `read` | `Calendars.Read.Shared` |
| Shared mounted calendar `manage` | `Calendars.ReadWrite.Shared` |
| Shared-mail `read` | `Mail.Read.Shared` |
| Shared-mail draft, filing, recovery, or deletion action | `Mail.ReadWrite.Shared` |
| Shared-mail `send` | `Mail.ReadWrite.Shared`, `Mail.Send.Shared` |
| Refresh tokens (always) | `offline_access` (added automatically by the identity library) |

`Mail.Send` is requested only when an account's own-mail policy enables `send`.

## Well-known client IDs

`OUTLOOK_MCP_CLIENT_ID` accepts friendly names in addition to raw UUIDs. Resolution is case-insensitive.

| Friendly name | Client ID | Application |
|---|---|---|
| `outlook-desktop` (default) | `d3590ed6-52b3-4102-aeff-aad2292ab01c` | Outlook desktop |
| `outlook-local-mcp` | `dd5fc5c5-eb9a-4f6f-97bd-1a9fecb277d3` | Outlook Local MCP (project app registration) |
| `teams-desktop` | `1fec8e78-bce4-4aaf-ab1b-5451cc387264` | Teams desktop and mobile |
| `teams-web` | `5e3ce6c0-2b1f-4285-8d4b-75ee78787346` | Teams web |
| `m365-web` | `4765445b-32c6-49b0-83e6-1d93765276ca` | Microsoft 365 web |
| `m365-desktop` | `0ec893e0-5785-4de6-99da-4ed124e5296c` | Microsoft 365 desktop |
| `m365-mobile` | `d3590ed6-52b3-4102-aeff-aad2292ab01c` | Microsoft 365 mobile |
| `outlook-web` | `bc59ab01-8403-45c6-8796-ac3ef710b3e3` | Outlook web |
| `outlook-mobile` | `27922004-5251-4030-b22d-91ecd9a37ea4` | Outlook mobile |

The default `outlook-desktop` client ID is pre-authorised for Graph Calendar scopes in all tenants — no admin consent required. If the value is not a recognised friendly name and does not look like a UUID, a warning is logged and the value is used as-is.

## In-server documentation surface

The server embeds its own documentation and exposes it through three verbs on the `system` domain:

```
{tool: "system", args: {operation: "list_docs"}}
{tool: "system", args: {operation: "search_docs", query: "token refresh"}}
{tool: "system", args: {operation: "get_docs", slug: "troubleshooting", section: "keychain-locked"}}
```

Each document is also exposed as an MCP resource at `doc://outlook-local-mcp/{slug}` for clients that support `resources/list` and `resources/read`. The server status response (`system.status`) includes a `docs` section with the base URI and the troubleshooting slug so an LLM client can locate the documentation surface without prior knowledge.

The embedded bundle contains exactly four slugs: `readme`, `quickstart`, `concepts` (this file), and `troubleshooting`.

## Observability at a glance

**Structured logging** — written to stderr. Configure with:

- `OUTLOOK_MCP_LOG_LEVEL` — minimum severity: `debug`, `info`, `warn`, `error` (default `warn`)
- `OUTLOOK_MCP_LOG_FORMAT` — `json` (default) or `text`
- `OUTLOOK_MCP_LOG_SANITIZE` — when `true` (default), PII such as email addresses and event body content is masked
- `OUTLOOK_MCP_LOG_FILE` — when set, log records are written to both stderr and the specified file (append mode, `0600` permissions)

**Audit logging** — when `OUTLOOK_MCP_AUDIT_LOG_ENABLED=true` (default), every tool invocation emits a structured JSON audit entry with the tool name, operation, and outcome. Set `OUTLOOK_MCP_AUDIT_LOG_PATH` to write entries to a file instead of stderr.

**OpenTelemetry** — optional OTLP gRPC export for metrics and traces:

```bash
OUTLOOK_MCP_OTEL_ENABLED=true \
OUTLOOK_MCP_OTEL_ENDPOINT=localhost:4317 \
./outlook-local-mcp
```

Metrics include per-tool invocation counts and durations. Traces create a span per tool invocation with tool name, parameters, and outcome attributes. Deep-dive OTel attribute lists and the full middleware chain are documented in `docs/reference/observability.md`.
