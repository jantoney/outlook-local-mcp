# Provision and verify the owned Entra application

Labels: `wayfinder:task`
Status: closed
Assignee: /root
Blocked by: none

## Question

Create the owned Entra public-client app with the decided audience and delegated permissions, then verify one organisational and one personal Microsoft account can consent and obtain Graph tokens through the MCP's chosen flow.

## Progress

The owned registration now supports all organisational directories and personal
Microsoft accounts, requests access-token version 2, and enables public-client
flows. Live verification exposed an MCP bootstrap-order defect: every aggregate
account verb is wrapped by the default-account authentication middleware, so a
disconnected default account configured for `auth_code` fails before
`account.add` can read its requested `auth_method="device_code"` override.

The immediate verification path is to restart the MCP with
`OUTLOOK_MCP_AUTH_METHOD=device_code`. The durable correction is tracked in the
new blocking ticket. This ticket remains open until one organisational and one
personal account complete consent and a Graph-backed identity call.

On 2026-07-20 the user authorized clearing all unused MCP account entries. The
persisted registry and four authentication records were moved into the
recoverable backup
`C:\Users\jayan\.outlook-local-mcp\backup-20260720-0526`; the active state
directory is otherwise empty and will recreate cleanly after restart.

## Resolution

The owned Entra public-client registration was corrected to support any Entra
tenant plus personal Microsoft accounts, access-token version 2, public-client
flows, client ID `e34d8306-27c5-4f59-b3cb-35583437f7e3`, and authority
`common`. The MCP was restarted with global `device_code` authentication after
the unused legacy account and authentication records were backed up.

Live organizational verification succeeded for account label `adelaiderep`,
which resolved to `jaya@adelaiderep.com`; a Graph `list_calendars` request
returned the account's calendars. Live personal verification succeeded for
account label `personal`, which resolved to `jay.antoney@outlook.com`; a second
Graph `list_calendars` request returned that account's calendars.

Both verification accounts intentionally used `calendar_only`, requesting
least-privilege consent for profile read, calendar read/write, and offline
access rather than every delegated permission present on the app registration.
The evidence establishes that the owned client, `common` authority, and device-
code flow work for both required account classes.

The test also exposed a separate bootstrap-order defect: aggregate account
verbs authenticate the implicit default before the selected handler can apply
its requested method. The global device-code restart allowed verification to
finish; the durable fix remains in `Fix account bootstrap authentication
ordering`.
