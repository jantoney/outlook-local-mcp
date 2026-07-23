---
name: per-instance-mcp-broker
description: Consolidate equivalent stdio launches behind one isolated local multi-session broker.
status: accepted
date: 2026-07-22
decision-makers: project maintainer
consulted: Outlook Local MCP implementers
informed: MCP harness users
source-branch: codex/shared-outlook-resources
source-commit: a635edd
---

# Use a Per-Instance Local MCP Broker

## Context and Problem Statement

MCP harnesses may start several copies of the stdio executable. Each current
process restores its own account registry, Graph clients, authentication state,
startup validation, logs, and optional web UI while sharing persistence. This
creates stale in-memory state and unsafe cross-process administration.

Equivalent launches need one authoritative runtime without accidentally joining
another installation or another state store. Account changes must become visible
to every connected harness without restarting Codex. Broker recovery must not
duplicate non-idempotent Graph operations.

## Decision Drivers

* One authoritative account registry and token-cache owner per compatible instance.
* Immediate visibility of web and MCP administration changes.
* Isolation by canonical executable and persistence identity.
* Independent MCP sessions and client capabilities for every harness.
* Safe recovery with automatic read-only replay and no automatic write replay.
* No network-accessible management endpoint.

## Considered Options

* Keep independent stdio processes and add filesystem watching.
* Make the first harness-owned stdio process the primary.
* Start a dedicated per-instance broker and use lightweight stdio proxies.
* Require users to configure one external Streamable HTTP server manually.

## Decision Outcome

Chosen option: "start a dedicated per-instance broker and use lightweight stdio
proxies", because it centralizes mutable state without tying all clients to the
lifetime of the first harness. The first launch starts the broker; every launch,
including the first, communicates with it through an isolated local transport.

The instance identity is a versioned hash of the canonical executable path,
state-realm identity, authentication-cache namespace, and behavior-affecting
configuration. A compatibility handshake must match the full unhashed descriptor
fields represented by that identity. Different executable locations or state
stores therefore do not join one another.

The broker owns MCP construction, persistence, account administration, Graph
clients, authentication, validation, observability, audit output, and the web UI.
Proxies own only their harness stdio stream and broker connection.

After broker loss, a proxy may automatically replay an interrupted operation
only when the authoritative per-verb metadata marks it read-only. Interrupted
write or destructive calls return a retry-required error to the agent and are
never automatically repeated.

### Consequences

* Good, because account and authentication changes are live across all clients.
* Good, because persistence becomes single-writer within an instance.
* Good, because harness lifecycle no longer owns authoritative runtime lifecycle.
* Good, because each client retains an independent MCP session.
* Good, because deployment and state fingerprints prevent cross-database routing.
* Bad, because startup adds broker election and a local proxy transport.
* Bad, because crash recovery cannot make an interrupted write transparent.
* Bad, because upgrades require broker protocol compatibility handling.

### Confirmation

Multi-process integration tests must prove single election, descriptor isolation,
live state propagation, per-client elicitation, broker recovery, read-only replay,
and write retry errors. Tests must also prove that `C:\outlook_mcp.exe` and
`D:\outlook_mcp.exe`, and two distinct state-store identities, never share a broker.

## Pros and Cons of the Options

### Independent processes with filesystem watching

* Good, because it changes little transport code.
* Bad, because token caches and authentication sessions remain multi-writer.
* Bad, because transient file state cannot provide one authoritative runtime.

### First harness-owned process as primary

* Good, because no additional broker process is visible.
* Bad, because closing that harness disconnects unrelated workflows.
* Bad, because the current stdio adapter assumes one static client session.

### Dedicated per-instance broker

* Good, because runtime ownership and lifecycle are explicit.
* Good, because a multi-session transport naturally isolates harness capabilities.
* Bad, because proxy and broker recovery logic must be maintained.

### Manually configured HTTP server

* Good, because MCP already standardizes Streamable HTTP.
* Bad, because every harness requires manual endpoint and security configuration.
* Bad, because it does not provide transparent stdio compatibility.

## More Information

This decision extends CR-0069. Authentication sessions remain process-bound;
the relevant process is now the isolated broker rather than an arbitrary stdio
proxy. The implementation uses `mark3labs/mcp-go` v0.45.0 multi-session
Streamable HTTP behavior as verified from the locally downloaded module source.
DeepWiki was listed as the preferred package reference but was unavailable in
the implementation session.
