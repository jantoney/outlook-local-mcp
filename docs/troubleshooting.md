# Troubleshooting Guide

Common failure modes and remediation steps for `outlook-local-mcp`.

---

## Authentication failures

**Symptom:** A tool call returns an error like `authentication required` or `failed to acquire token`.

**Cause:** No token has been cached for the account, or the cached token has expired.

**Remediation:**

1. Call `{tool: "account", args: {operation: "list"}}` to see which accounts are registered and which are disconnected.
2. If the account shows `disconnected`, call `{tool: "account", args: {operation: "login", label: "<label>"}}` to re-authenticate using the account's persisted method.
3. If no account is registered at all, call `{tool: "account", args: {operation: "add", label: "default"}}` to register and authenticate the first account.
4. After successful login, retry the original tool call. The `AuthMiddleware` will automatically retry once authentication completes.

---

## Token refresh

**Symptom:** A tool call returns an error referencing `token expired`, `refresh token`, or `invalid_grant`.

**Cause:** The OAuth refresh token expired (typically after 90 days of inactivity or after an Entra ID policy change) or the token cache was corrupted.

**Remediation:**

1. Call `{tool: "account", args: {operation: "refresh", label: "<label>"}}` to force a silent token refresh. This succeeds when the refresh token is still valid.
2. If the refresh fails, call `{tool: "account", args: {operation: "login", label: "<label>"}}` to initiate a new interactive authentication flow.
3. After re-authenticating, the server persists a new token and automatically retries the pending tool call.

**Prevention:** The server performs a silent token probe at startup. `device_code` accounts skip the probe to avoid crash loops; other methods probe silently and pre-cache tokens before the first tool call.

---

## Device code flow

**Symptom:** Authentication returns a device code URL and code in the tool result text instead of completing automatically.

**Cause:** The MCP client does not support the Elicitation API (e.g., Claude Code). The server falls back to returning the device code directly in the tool result.

**Remediation:**

1. Copy the URL and code from the tool result.
2. Open the URL in a browser, enter the code, and complete the Microsoft sign-in.
3. After sign-in, call any tool again. The server picks up the cached token automatically.

---

## Browser auth flow

**Symptom:** A tool call returns an error like `browser authentication timed out` or the browser window did not appear.

**Cause:** The server opened the system browser for OAuth login but the user did not complete sign-in within the timeout, or the browser did not open (headless environment).

**Remediation:**

1. Retry the tool call to trigger a fresh browser auth attempt.
2. If the browser does not open automatically, use `auth_code` method instead: set `OUTLOOK_MCP_AUTH_METHOD=auth_code` and restart the server.
3. For headless environments (containers, SSH), use `device_code` authentication: set `OUTLOOK_MCP_AUTH_METHOD=device_code`.

---

## Auth code flow

**Symptom:** The server returns an auth URL and asks you to paste the redirect URL back.

**Cause:** The `auth_code` method was selected. After signing in, the browser redirects to the `nativeclient` URI and shows the full redirect URL in the address bar.

**Remediation:**

1. Copy the full redirect URL from the browser address bar after sign-in.
2. If the MCP client supports Elicitation, paste it into the prompt.
3. If not, call `{tool: "system", args: {operation: "complete_auth", redirect_url: "<url>"}}` to exchange the code for a token.

---

## Keychain locked

**Symptom:** Token storage fails with an error referencing `keychain`, `SecKeychain`, `libsecret`, or `DPAPI`.

**Cause:** The OS keychain is locked, unavailable, or the process lacks permission to access it (common in headless, container, or screen-locked environments).

**Remediation:**

1. Unlock the keychain (macOS: open **Keychain Access** and unlock the login keychain; Linux: ensure `gnome-keyring` or `kwallet` is running).
2. If the keychain cannot be unlocked, set `OUTLOOK_MCP_TOKEN_STORAGE=file` in the server configuration to switch to the file-based AES-256-GCM encrypted token cache at `~/.outlook-local-mcp/token_cache.bin`.
3. Restart the server after changing `OUTLOOK_MCP_TOKEN_STORAGE`.
4. Container and Docker builds (`CGO_ENABLED=0`) always use the file-based cache regardless of `TOKEN_STORAGE` setting; no action required.

**Note:** When `TOKEN_STORAGE=auto` (the default) and the keychain is unavailable, the server automatically falls back to the file cache. Only `TOKEN_STORAGE=keychain` (no fallback) will error in this scenario.

---

## Multi-account resolution

**Symptom:** A tool call errors with `multiple authenticated accounts` or the wrong account is used.

**Cause:** Multiple accounts are authenticated and no `account` parameter was supplied.

**Remediation:**

1. Call `{tool: "account", args: {operation: "list"}}` to see all registered accounts with their labels and UPNs.
2. Pass the `account` parameter to any tool: `{tool: "calendar", args: {operation: "list_events", account: "work"}}`. Both label and UPN (e.g., `alice@contoso.com`) are accepted.
3. If the MCP client supports Elicitation, a prompt appears for account selection when no `account` param is supplied; select the desired account from the list.

**Account states:**

- `authenticated`: Token is cached and valid.
- `disconnected`: Account is registered but the token expired or was cleared. Call `account.login` to reconnect.

---

## Graph 429 throttling

**Symptom:** A tool call returns an error like `Graph API error [TooManyRequests]: 429` or `ApplicationThrottled`.

**Cause:** The Microsoft Graph API is rate-limiting requests from this application or tenant.

**Remediation:**

1. Wait a few seconds and retry the tool call. The server applies automatic exponential backoff for 429 responses (configured via `OUTLOOK_MCP_MAX_RETRIES` and `OUTLOOK_MCP_RETRY_BACKOFF_MS`).
2. If throttling persists, increase `OUTLOOK_MCP_RETRY_BACKOFF_MS` (default 1000ms) to give Graph more recovery time between retries.
3. Avoid tight loops of rapid tool calls, especially for calendar listing operations across large date ranges.

**Configuration:** `OUTLOOK_MCP_MAX_RETRIES` (default 3) and `OUTLOOK_MCP_RETRY_BACKOFF_MS` (default 1000) control the retry behaviour. Check the current values with `{tool: "system", args: {operation: "status", output: "summary"}}` under `config.graph_api`.

---

## Inefficient filter

**Symptom:** A tool call returns an error like `Graph API error [ErrorInvalidRequest]` or a message referencing `InefficientFilter` or missing `$orderby` constraints.

**Cause:** The Microsoft Graph `$filter` query contains a condition on a non-indexed field without the required `$orderby` clause, or uses a filter expression that Graph does not support server-side.

**Remediation:**

1. Avoid filtering on non-indexed properties. Graph indexes `subject`, `start/dateTime`, `end/dateTime`, `organizer/emailAddress/address`, and `isOrganizer` for calendar queries.
2. When using `$filter` on `start/dateTime` or `end/dateTime`, include `$orderby=start/dateTime` (or `end/dateTime`) in the same request.
3. For mail queries, use the `search` parameter (KQL syntax) instead of `$filter` for full-text search. `$filter` on mail supports `isRead`, `isDraft`, `hasAttachments`, `importance`, `flag/flagStatus`, and `receivedDateTime`.
4. If the error persists, use `output=raw` on the offending tool to see the full OData error detail, then adjust the filter expression.

---

## Insufficient mail capability

**Symptom:** A mail verb reports that the selected account's own-mail policy disables the required capability.

**Cause:** All mail verbs are discoverable, but the selected account's exact action switch is off. OAuth consent does not override this local policy.

**Remediation:**

1. Check the action matrix with `{tool: "account", args: {operation: "list"}}`.
2. Change only the intended account, for example `{tool: "account", args: {operation: "set_mail_policy", label: "work", read: true, draft: true}}`.
3. If the response says OAuth scopes changed, reconnect it with `account.login`. Same-scope changes apply without disconnecting.

---

## Local attachment upload disabled

**Symptom:** `mail.add_attachment` reports that local attachment upload is disabled or the path is outside configured roots.

**Cause:** `OUTLOOK_MCP_ATTACHMENT_ROOTS` is empty, the account's `draft` capability is off, or canonical path resolution places the file outside every allowed root.

**Remediation:**

1. Configure `OUTLOOK_MCP_ATTACHMENT_ROOTS` as an OS path-list of the smallest directories that contain intended files, then restart.
2. Confirm the selected account's own-mail policy enables `draft`.
3. Use a regular file under an allowed root; traversal and symlink/junction escapes are intentionally rejected.

---

## Revoke Microsoft app consent

**Symptom:** Mail actions were disabled locally, but Microsoft still records earlier delegated consent.

**Cause:** `account.set_mail_policy` enforces the local matrix immediately and clears local tokens when required scopes change, but it cannot revoke a grant stored by Microsoft.

**Remediation:** Remove the application's consent from the personal Microsoft account privacy/app permissions page, or have an Entra administrator revoke the enterprise application's user/admin consent. Then reconnect the account and consent only to the scopes required by the current action policy.

---

## Read-only mode

**Symptom:** Write tool calls (create, update, delete, cancel, draft operations) return `server is in read-only mode`.

**Cause:** `OUTLOOK_MCP_READ_ONLY=true` is set. In read-only mode all write operations are blocked at the `ReadOnlyGuard` middleware before reaching the handler.

**Remediation:**

1. If read-only mode is intentional (e.g., a shared or supervised environment), do not change this setting. Only read verbs (`list_*`, `get_*`, `search_*`, `status`) are available.
2. To re-enable writes, remove or set `OUTLOOK_MCP_READ_ONLY=false` and restart the server.
3. Verify the current mode with `{tool: "system", args: {operation: "status"}}` and look for `read-only=on` in the Features line.

---

## Log file location

Logs are written to the path configured in `OUTLOOK_MCP_LOG_FILE`. By default, no file logging is active and logs are written only to stderr (which the MCP client typically captures but does not surface in the chat).

To enable file logging:

1. Set `OUTLOOK_MCP_LOG_FILE=/path/to/outlook-local-mcp.log` in the server environment.
2. Set `OUTLOOK_MCP_LOG_LEVEL=debug` for verbose output during troubleshooting.
3. Restart the server.

The log file path and current level can be read with `{tool: "system", args: {operation: "status", output: "summary"}}` under `config.logging.log_file` and `config.logging.log_level`.

**PII sanitization:** When `OUTLOOK_MCP_LOG_SANITIZE=true` (the default), email addresses and other identifiers are replaced with redacted placeholders in all log output. Set to `false` only in controlled debugging sessions.

**Audit log:** When `OUTLOOK_MCP_AUDIT_LOG_ENABLED=true`, a structured audit trail is written to `OUTLOOK_MCP_AUDIT_LOG_PATH` (defaults to a file alongside the main log). The audit log records every tool invocation with timestamp, operation name, account label, and outcome.

---

## Account lifecycle

The `account` domain tool manages the full lifecycle of Microsoft accounts registered with the server.

### Add an account

```
{tool: "account", args: {operation: "add", label: "work"}}
```

Registers and authenticates a new account. Optional parameters: `client_id`, `tenant_id`, `auth_method`. The UPN (`alice@contoso.com`) is resolved from Graph `/me` and persisted to `accounts.json` after successful authentication.

### List accounts

```
{tool: "account", args: {operation: "list"}}
```

Returns all registered accounts with label, UPN, authentication state, and auth method. Disconnected accounts (expired tokens) are shown as first-class entries, not hidden.

### Log in a disconnected account

```
{tool: "account", args: {operation: "login", label: "work"}}
```

Re-authenticates a disconnected account using its persisted `auth_method`, `client_id`, and `tenant_id`. Errors if the account is already connected.

### Log out an account

```
{tool: "account", args: {operation: "logout", label: "work"}}
```

Disconnects an account without removing its configuration from `accounts.json`. The account remains visible as `disconnected` and can be reconnected via `account.login`.

### Force token refresh

```
{tool: "account", args: {operation: "refresh", label: "work"}}
```

Forces a silent token refresh (`ForceRefresh=true`). Returns the new token expiry. Useful after an Entra ID permission change or when token staleness is suspected.

### Remove an account

```
{tool: "account", args: {operation: "remove", label: "work"}}
```

Permanently removes the account from the registry, clears its keychain cache, and rewrites `accounts.json` (atomic temp-file + rename) without the removed entry so the removal survives a restart (see CR-0064). When `accounts.json` has no entry for the label (for example the implicit `default` created from env config), the in-memory removal still succeeds for the current session but the entry reappears at the next start. Use `account.logout` instead when you want to disconnect without deleting the configuration. See [Auto-default account](#auto-default-account) for the implicit-default gating rule.

---

## Auto-default account {#auto-default-account}

The implicit `default` account registration is conditional on `accounts.json` contents, and `account.remove` is persistent across restart (see CR-0064).

### Ghost-default scenario

On startup, the server registers an implicit "default" account from the env config (`OUTLOOK_MCP_CLIENT_ID`, `OUTLOOK_MCP_TENANT_ID`, `OUTLOOK_MCP_AUTH_METHOD`) when no entry in `accounts.json` already covers that identity. If the keychain or file token cache has been cleared for that identity, `account_list` shows a disconnected `default` entry. The first tool call that falls through to "default" will trigger an authentication prompt (device-code or browser), which may be unexpected in a multi-account setup.

**Remedy:** Add an entry to `accounts.json` whose `client_id` and `tenant_id` match the env config, or use `account_remove default` to remove the ghost entry for the current session.

### Persistent-removal semantics

`account_remove` is durable across restarts when `accounts.json` contains an entry for the removed label. After removal, the server rewrites `accounts.json` without that entry. The removed account does not return on the next start.

When `accounts.json` has no entry for the label (for example, the implicit "default" created from env config without any `accounts.json`), the in-memory removal still succeeds for the current session, but the entry reappears at the next start because `main.go` re-registers it from the env config.

### accounts.json gating rule

The implicit "default" is skipped at startup when either of these is true:

- `accounts.json` contains an entry whose `client_id` and `tenant_id` match the env config, regardless of that entry's label.
- `accounts.json` contains any entry with the literal label `"default"`.

When neither is true (for example, `accounts.json` is absent or empty), the implicit "default" is registered so that single-account env-only setups continue to work without authoring a config file.

### Removing a cfg-identity-covering entry causes default to reappear

If `accounts.json` has a single entry whose `client_id` and `tenant_id` match the env config, removing that entry causes the implicit "default" to reappear at the next start. This is intentional: removing the entry from `accounts.json` removes the gating signal, so `main.go` falls back to the implicit default. To suppress the implicit default permanently, keep an `accounts.json` entry that covers the cfg identity under any label.

---

## Mounted calendar missing {#mounted-calendar-missing}

If a configured mounted calendar ID is missing or Graph rejects its route, list, search, and get return reselection guidance after the one exact mounted request. The server does not retry through the owner, default `/me` calendar, display-name search, or another mounted ID. This protects mailbox-view-scoped event identifiers from being replayed against the wrong mount.

**Remedy:** Call `account.discover_calendar_aliases` with the same account and owner. Have the user select the intended calendar, then call `account.reselect_calendar_alias` with the exact `mounted_calendar_id` and `confirm_mounted_selection=true`. Reselection preserves the alias's immutable resource identity; changing the owner or kind requires remove and recreate.

---

## Shared calendar reference is rejected {#shared-calendar-reference-rejected}

A shared event reference is valid only for the account, immutable calendar resource, mailbox view, and event from which it was issued. It also stops resolving when the alias is removed and recreated, even if the same alias text is reused. Passing `event_id` with `shared_resource`, changing aliases, or altering the signed value fails locally before Graph.

**Remedy:** Confirm the intended account and alias still exist with `account.list_calendar_aliases`, then call `calendar.list_events` or `calendar.search_events` again for that exact target and use the newly returned `resource_ref`. If Graph denies the owner route, verify the owner shared or delegated the primary calendar to the signed-in organizational account; OAuth consent alone does not create Exchange access.

## Mounted calendar write is rejected {#mounted-calendar-write-rejected}

Mounted create, update, reschedule, and delete require a validated organizational account, a mounted alias with profile `manage`, an editable Exchange mount, and read-only mode disabled. Follow-up writes also require a current `resource_ref`. Eligible events must have zero attendees and explicitly report `isOnlineMeeting=false`; missing classification, attendee-bearing events, and online-enabled events are rejected before mutation. Mounted create and update cannot enable online meetings. Shared response and cancellation operations are not supported. A local policy, alias, connection, or route change after preflight also stops the pending mutation.

**Remedy:** Confirm the account and alias with `account.list_calendar_aliases`, restore the intended manage profile if appropriate, reconnect when the scope union changed, and obtain a fresh reference from `calendar.list_events` or `calendar.search_events`. Manage attendee-bearing or online-enabled meetings from the organizer's own calendar with the meeting-specific verbs. Do not remove attendees or attempt to set `is_online_meeting=false` merely to bypass the boundary.

---

## Shared mailbox is incompatible {#shared-mail-incompatible}

Shared-mail configuration and use require authoritative token evidence for an organizational Microsoft 365 tenant. Personal Microsoft accounts and unknown token contexts fail closed before Graph traffic, regardless of granted OAuth scopes.

**Remedy:** Confirm `token_tenant_context` with `account.list` or `system.status`. Reconnect the intended work or school account if validated tenant evidence is missing. Configure the alias under that account and separately verify Exchange Full Access, folder delegation, or Send As/Send on Behalf rights as required. Removing an invalid local alias remains available without Graph traffic.

## Shared mail reference is rejected {#shared-mail-reference-rejected}

A shared folder or message reference is valid only for the account, immutable mailbox resource, owner view, item kind, and Graph ID from which it was issued. Raw `folder_id` or `message_id` plus `shared_resource`, cross-mailbox references, disabled read policy, removed aliases, and recreated aliases fail locally before Graph.

**Remedy:** Confirm the organizational account and alias with `account.list_mail_aliases`, ensure only the intended alias has `read` enabled, and obtain fresh references from `mail.list_folders` or `mail.list_messages` on that exact alias. If the exact owner route is denied, verify Exchange mailbox or folder delegation separately; broader OAuth consent does not create mailbox access.

---

## In-server documentation access

The server embeds this guide and other user-facing documentation. The LLM can access it directly without leaving the session:

```
{tool: "system", args: {operation: "list_docs"}}
{tool: "system", args: {operation: "search_docs", query: "token refresh"}}
{tool: "system", args: {operation: "get_docs", slug: "troubleshooting", section: "token-refresh"}}
```

Each embedded document is also available as an MCP resource at `doc://outlook-local-mcp/{slug}` (e.g., `doc://outlook-local-mcp/troubleshooting`). LLM clients that support `resources/list` and `resources/read` can fetch documents natively.
