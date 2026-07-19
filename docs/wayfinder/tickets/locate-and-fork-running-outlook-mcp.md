# Locate and fork the running Outlook MCP

Labels: `wayfinder:task`
Status: closed

## Question

Identify the exact source and release of the running Outlook MCP, create a source checkout, and establish a user-owned GitHub fork so later decisions refer to a concrete codebase.

## Resolution

Codex configuration launches `C:\Users\jayan\Documents\Codex\outlook-local-mcp\outlook-local-mcp.exe`. The installed executable and ZIP are dated 2026-04-26, matching upstream release v0.4.0. The upstream repository is <https://github.com/desek/outlook-local-mcp>, and its own documentation confirms it already uses Microsoft Graph.

The current upstream source was cloned into `D:\DevWork\OutlookMCP`. A GitHub fork was created at <https://github.com/jantoney/outlook-local-mcp> and added locally as remote `fork`; upstream remains remote `origin`.
