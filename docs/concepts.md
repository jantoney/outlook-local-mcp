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

An `owner_primary_calendar` identifies the owner's primary calendar in owner view. It is available only with an organizational token context and remains read-only. A `mounted_calendar` identifies one calendar in the signed-in recipient's view. Creation and reselection perform fresh `/me/calendars` discovery filtered by the configured owner and require the human to confirm the exact mounted calendar ID. Mounted manage requires an organizational token context and an editable discovery result.

Use `account.discover_calendar_aliases`, then `account.add_calendar_alias`. Listing shows immutable identity and exact policy. Renaming changes only the human selector. Reselection changes only the mounted ID after fresh confirmation. Owner, kind, and mailbox view cannot be edited; retargeting requires idempotent removal and recreation, which creates a new resource ID. A missing mount is never silently rebound.

Shared calendar read contributes `Calendars.Read.Shared`; manage contributes `Calendars.ReadWrite.Shared`. These join the account-wide OAuth scope union but do not replace target-local authorization or Exchange sharing rights.

## Shared mail aliases

A shared-mail alias is a separate account-scoped allowlist entry for one owner-view mailbox routed through `/users/{owner}/...`. It has an immutable resource ID, owner, `mailbox` kind, and `owner` mailbox view. Renaming changes only the human selector and preserves identity. Retargeting requires idempotent removal and recreation, which creates a new identity and invalidates old target-bound references. A calendar and mail alias may use the same selector and owner without becoming the same resource.

New aliases start with all eight actions disabled: `read`, `draft`, `move`, `archive`, `trash`, `restore`, `permanent_delete`, and `send`. Use `account.set_mail_alias_policy` to change only explicitly supplied switches. Each alias policy is independent of the signed-in account's own-mail policy and every other alias. Personal and unknown token contexts reject shared-mail configuration and use locally; only validated organizational contexts are compatible. Exchange mailbox, folder, and Send As or Send on Behalf delegation remains authoritative.

Shared read contributes `Mail.Read.Shared`. Draft, filing, recovery, deletion, or send contributes `Mail.ReadWrite.Shared`; send also contributes `Mail.Send.Shared`. The server disconnects the account only when changing an alias alters the complete account-wide scope union. OAuth consent never authorizes an action disabled by the selected alias policy.

## Local draft attachments

`mail.add_attachment` attaches exactly one local file to an existing draft and requires the exact `draft` capability. `OUTLOOK_MCP_ATTACHMENT_ROOTS` is a platform path-list allowlist; an empty value disables local upload. Canonical paths outside those roots, including traversal and link escapes, are rejected. Files below 3 MiB use direct upload, files through 150 MiB use sequential resumable upload, and larger files are rejected.

## Confirmed draft send

`mail.send_draft` requires the exact `send` capability and accepts only an existing draft ID. Draft capability does not imply send. The server fetches the subject, recipients, and attachment names and presents them through MCP elicitation. It sends only after the human explicitly accepts; unsupported elicitation, decline, and cancel all fail safely. Graph acceptance does not confirm final delivery, which remains subject to Exchange processing.

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
