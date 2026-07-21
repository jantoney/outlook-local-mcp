# Quick Start

Get from zero to a working Outlook Local MCP server in minutes.

## Prerequisites

- **Go 1.24+** installed ([download](https://go.dev/dl/))
- A **Microsoft account** (personal, work, or school)

## 1. Build

```bash
git clone https://github.com/desek/outlook-local-mcp.git
cd outlook-local-mcp
go build ./cmd/outlook-local-mcp/
```

Or install directly:

```bash
go install github.com/desek/outlook-local-mcp/cmd/outlook-local-mcp@latest
```

## 2. Configure Claude Desktop

Add the server to your Claude Desktop configuration file:

**macOS**: `~/Library/Application Support/Claude/claude_desktop_config.json`
**Windows**: `%APPDATA%\Claude\claude_desktop_config.json`

```json
{
  "mcpServers": {
    "outlook-local": {
      "command": "/absolute/path/to/outlook-local-mcp"
    }
  }
}
```

Replace `/absolute/path/to/outlook-local-mcp` with the actual path to the built binary.

To set environment variables:

```json
{
  "mcpServers": {
    "outlook-local": {
      "command": "/absolute/path/to/outlook-local-mcp",
      "env": {
        "OUTLOOK_MCP_DEFAULT_TIMEZONE": "America/New_York",
        "OUTLOOK_MCP_LOG_LEVEL": "info"
      }
    }
  }
}
```

## 2b. Configure Claude Code

Add an `.mcp.json` file to your project root:

```json
{
  "mcpServers": {
    "outlook-local": {
      "command": "/absolute/path/to/outlook-local-mcp"
    }
  }
}
```

Replace `/absolute/path/to/outlook-local-mcp` with the actual path to the built binary.

## 3. Authenticate and Verify

Restart Claude Desktop (or reload MCP servers in Claude Code) and ask:

> "List my calendars"

On first use, the server has no cached credentials. The default authentication method (`auth_code`) opens the system browser for Microsoft login. After signing in, the browser shows a blank page -- copy the full URL from the address bar and paste it when prompted (via MCP Elicitation) or use the `complete_auth` tool if your client does not support elicitation (e.g., Claude Code). After authentication completes, the tool call is retried automatically and your calendars are returned. Tokens are cached in your OS keychain -- subsequent requests authenticate silently.

## 4. Tool Examples

### Read

**List calendars** -- no parameters required:
> "Show me all my calendars"

**List events** in a time range:
> "What meetings do I have tomorrow?"

Parameters: `start_datetime` (required), `end_datetime` (required), `calendar_id`, `max_results`, `timezone`.

**Get event** details by ID:
> "Get the full details of event AAMkAD..."

Parameters: `event_id` (required), `timezone`.

### Search

**Search events** by subject, importance, sensitivity, and more:
> "Find all high-importance meetings in the next two weeks"

Parameters: `query`, `start_datetime`, `end_datetime`, `importance`, `sensitivity`, `is_all_day`, `show_as`, `is_cancelled`, `categories`, `max_results`, `timezone`. All optional; defaults to next 30 days.

**Free/busy** availability:
> "When am I free next Monday?"

Parameters: `start_datetime` (required), `end_datetime` (required), `timezone`.

### Write

**Create event**:
> "Schedule a team standup tomorrow at 9 AM Eastern for 30 minutes with alice@example.com"

Required: `subject`, `start_datetime`, `start_timezone`, `end_datetime`, `end_timezone`.
Optional: `body`, `location`, `attendees` (JSON array), `is_online_meeting`, `is_all_day`, `importance`, `sensitivity`, `show_as`, `categories`, `recurrence` (JSON object), `reminder_minutes`, `calendar_id`.

**Update event** -- only specified fields change (PATCH semantics):
> "Move my 2pm meeting to 3pm"

Required: `event_id`. All other fields are optional.

### Delete

**Delete event**:
> "Delete the event AAMkAD..."

Parameters: `event_id` (required). Cancellation notices are sent to attendees automatically if you are the organizer.

**Cancel event** with a message to attendees:
> "Cancel tomorrow's team meeting and let everyone know it's rescheduled"

Parameters: `event_id` (required), `comment` (optional cancellation message). Only the organizer can cancel.

## 5. Configure a shared calendar alias

First ensure the owner has shared or delegated the calendar to the signed-in account in Outlook or Exchange. OAuth consent alone does not grant calendar access.

For a mounted calendar, discover recipient-view candidates and have the user select the exact ID:

```json
{"tool":"account","args":{"operation":"discover_calendar_aliases","label":"work","owner":"owner@contoso.com"}}
{"tool":"account","args":{"operation":"add_calendar_alias","label":"work","alias":"finance-calendar","owner":"owner@contoso.com","kind":"mounted_calendar","profile":"read","mounted_calendar_id":"<selected-id>","confirm_mounted_selection":true}}
```

Mounted list, search, and referenced get calls use that exact selected ID beneath `/me/calendars`; the server never builds the route from the configured owner. Unknown token tenant context is rejected until validated personal or organizational evidence is available.

For an organizational owner's primary calendar, create an `owner_primary_calendar` with profile `off` or `read`. Owner-primary manage is not supported. Adding a shared scope disconnects the account only when the effective scope union changes; call `account.login` when prompted.

After adding a read-enabled owner-primary alias, list events and retain the returned signed reference for follow-up reads:

```json
{"tool":"calendar","args":{"operation":"list_events","account":"work","shared_resource":"finance-calendar","date":"this_week","output":"summary"}}
{"tool":"calendar","args":{"operation":"get_event","account":"work","shared_resource":"finance-calendar","resource_ref":"<returned-resource-ref>"}}
```

Do not pass a shared event's raw Graph ID to `get_event`; shared follow-ups require the target-bound reference. Omitting `shared_resource` keeps the existing own-calendar workflow.

## 6. Configure a shared mailbox alias

Shared mailboxes require a validated organizational token context and existing Exchange delegation. Add the immutable owner-view identity first; every action starts disabled:

```json
{"tool":"account","args":{"operation":"add_mail_alias","label":"work","alias":"finance-mail","owner":"finance@contoso.com"}}
{"tool":"account","args":{"operation":"set_mail_alias_policy","label":"work","alias":"finance-mail","read":true,"archive":true}}
```

Call `account.list_mail_aliases` to review the exact target policy and compatibility. Enabling shared send additionally requires Exchange Send As or Send on Behalf rights and the separate confirmed-draft workflow.

Shared draft attachments are limited to allowlisted files smaller than 3 MiB. Large shared upload sessions are unsupported because the required live organizational-mailbox compatibility evidence is unavailable; own-mail resumable attachments remain supported through 150 MiB.

## 7. Configuration

All environment variables are prefixed with `OUTLOOK_MCP_`:

| Variable | Default | Description |
|---|---|---|
| `CLIENT_ID` | Microsoft Office client ID | OAuth 2.0 client ID |
| `TENANT_ID` | `common` | Entra ID tenant (`common`, `organizations`, `consumers`, or a GUID) |
| `DEFAULT_TIMEZONE` | `UTC` | IANA timezone for calendar operations |
| `LOG_LEVEL` | `warn` | Log level: `debug`, `info`, `warn`, `error` |
| `READ_ONLY` | `false` | Disable write tools (create, update, delete, cancel) |
| `MAIL_ENABLED` | `false` | Legacy/implicit default mapping to own-mail `read` |
| `MAIL_MANAGE_ENABLED` | `false` | Legacy/implicit default mapping to own-mail `read` and `draft` |
| `MAIL_SEND_ENABLED` | `false` | Legacy/implicit default mapping to own-mail `read`, `draft`, and `send` |
| `ATTACHMENT_ROOTS` | *(empty)* | Platform path-list of directories allowed for local draft attachments |
| `LOG_FORMAT` | `json` | Log format: `json` or `text` |
| `LOG_SANITIZE` | `true` | Mask PII in log output |
| `LOG_FILE` | *(empty = disabled)* | Log file path for persistent file output |
| `ACCOUNTS_PATH` | `~/.outlook-local-mcp/accounts.json` | Path to the persistent accounts file for multi-account support (see CR-0032) |

### Using an app registration you own

For predictable work, school, and personal account support, create a Microsoft identity platform public-client registration with **Accounts in any organizational directory and personal Microsoft accounts** (`AzureADandPersonalMicrosoftAccount`) and access-token version 2. Add the delegated Graph permissions `User.Read` and `Calendars.ReadWrite`, plus only the mail permissions required by the action policies you will use (`Mail.Read`, `Mail.ReadWrite`, and optionally `Mail.Send`). Enable public-client flows, add the mobile/desktop redirect URIs `http://localhost` and `https://login.microsoftonline.com/common/oauth2/nativeclient`, and do not create a client secret. Set `OUTLOOK_MCP_CLIENT_ID` to that application ID and keep `OUTLOOK_MCP_TENANT_ID=common` to allow both organizational and personal accounts.

## Getting help in-session

The server embeds its own documentation so the LLM can look up answers without leaving the conversation.

List available documents:

```
{tool: "system", args: {operation: "list_docs"}}
```

Search across all embedded docs:

```
{tool: "system", args: {operation: "search_docs", query: "token refresh"}}
```

Fetch a document or a specific section by heading anchor:

```
{tool: "system", args: {operation: "get_docs", slug: "troubleshooting"}}
{tool: "system", args: {operation: "get_docs", slug: "troubleshooting", section: "keychain-locked"}}
```

The embedded bundle contains `readme`, `quickstart`, and `troubleshooting`. Each document is also exposed as an MCP resource at `doc://outlook-local-mcp/{slug}` for clients that support `resources/list` and `resources/read`. Run `system.status` to discover the base URI and the troubleshooting slug. See CR-0061 for implementation details.

## Further Reading

See [README.md](README.md) for the full reference documentation.
