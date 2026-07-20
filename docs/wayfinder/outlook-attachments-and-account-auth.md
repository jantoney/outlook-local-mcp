# Outlook attachments and account authentication

Labels: `wayfinder:map`

## Destination

An approved, implementation-ready Change Request that specifies local-file attachments for Outlook drafts and reliable delegated authentication for work, school, and personal Microsoft accounts in the `jantoney/outlook-local-mcp` fork.

## Notes

- Source checkout: `desek/outlook-local-mcp` at `5b22fb5`.
- Fork: <https://github.com/jantoney/outlook-local-mcp>.
- The installed v0.4.0 binary is Graph-based already; re-platforming to Graph is not required.
- Follow `AGENTS.md`, the repository governance skill, and CR lifecycle rules.
- Wayfinder is planning-only for this effort. Implementation begins after the proposed CR is reviewed and approved.
- Microsoft Graph and Microsoft identity-platform documentation are authoritative. DeepWiki was unavailable in the session that charted this map.

## Decisions so far

- [Locate and fork the running Outlook MCP](tickets/locate-and-fork-running-outlook-mcp.md) — the live binary is upstream v0.4.0 from `desek/outlook-local-mcp`; source is cloned and forked to `jantoney/outlook-local-mcp`.
- [Choose the owned Entra application authentication contract](tickets/choose-owned-entra-app-authentication-contract.md) — use an owned secretless public client for organisational and personal accounts, tenant `common`, delegated Graph permissions including `User.Read`, and explicit v2-token/public-client settings.
- [Choose the Graph attachment upload contract](tickets/choose-graph-attachment-upload-contract.md) — one draft-only `mail.add_attachment` verb selects direct POST below 3 MiB and a resumable upload session from 3 MiB through 150 MiB, with local cancellation-aware upload handling.
- [Choose the MCP local-file trust boundary](tickets/choose-mcp-local-file-trust-boundary.md) — accept one path within explicitly configured canonical attachment roots; upload is disabled when no roots are configured.
- [Choose the upstream baseline and compatibility target](tickets/choose-upstream-baseline-and-compatibility-target.md) — build from fork `main` at `5b22fb5`, matching current upstream `main` rather than preserving the installed v0.4.0 surface.
- [Choose per-account mail risk profiles](tickets/choose-per-account-mail-risk-profiles.md) — use cumulative account profiles from calendar-only through elicitation-confirmed draft sending; `Mail.Send` is requested only for `mail_send` accounts.
- [Provision and verify the owned Entra application](tickets/provision-and-verify-owned-entra-application.md) — the owned `common` public client successfully authenticated one organizational and one personal account through device code and completed Graph calendar reads with least-privilege `calendar_only` consent.

## Not yet specified

None. Live verification made the remaining implementation questions precise;
they now live in open child tickets.

## Out of scope

- Sending messages automatically; the existing draft-only safety boundary remains.
- Application-permission or daemon access to personal mailboxes; the design uses delegated user authentication.
- Replacing Microsoft Graph with Outlook COM, EWS, IMAP, or SMTP.
- Adding attachments to sent or received messages.
