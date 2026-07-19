# Choose per-account mail risk profiles

Labels: `wayfinder:grilling`
Status: closed

## Question

How should the MCP let the user accept different mail permission risks for different connected Microsoft accounts, including optional `Mail.Send`?

## Resolution

Use cumulative per-account profiles: `calendar_only`, `mail_read`,
`mail_manage`, and `mail_send`. Draft-only `mail_manage` remains the default
when the existing global mail-management flag is enabled. Only `mail_send`
requests `Mail.Send`, and its initial tool surface is limited to sending an
existing draft after human MCP elicitation.

Global flags remain backward-compatible defaults. Account-specific profiles
are persisted, used to construct account-specific Graph clients, displayed in
status, and enforced at runtime before Graph calls.
