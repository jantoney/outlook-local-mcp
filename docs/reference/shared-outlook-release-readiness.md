# Shared Outlook release readiness

## Decision

**NO-GO as of 2026-07-21.** Issue #22 is not complete, and publication work in
issue #23 must not begin. The automated Go suite passes, but the required live
tenant compatibility gates were not run because no representative delegated
organizational mailbox, mounted calendar, Exchange-rights fixture, or disposable
test data was available.

This report records evidence without broadening the supported surface. It does
not treat skipped live scenarios as successful validation.

## Automated evidence

| Gate | Result | Evidence or blocker |
| --- | --- | --- |
| Embedded documentation | PASS | `make ci` completed `docs-bundle`, including slug resolution, the 2 MiB budget, secret-pattern lint, `llms.txt` generation, and catalog matching. |
| Build | PASS | `make ci` completed `go build ./cmd/outlook-local-mcp/`. |
| Vet | PASS | `make ci` completed `go vet ./...`. |
| Full test and coverage suite | PASS | `make test` completed as an unprivileged container user; every package passed. This execution context is required by the atomic-write permission-denial test. |
| Full race suite | PASS | `go test -race ./... -count=1` completed as an unprivileged container user; every package passed. New Graph SDK fixtures were serialized because client construction toggles package-global backing-store state. |
| Focused shared-resource suites | PASS | `go test ./internal/resource ./internal/auth ./internal/audit ./internal/tools ./internal/server` passed after the shared-send and upload-boundary changes. |
| Formatting | BLOCKED BY BASELINE | Linux `gofmt -l .` reports existing CRLF-formatted repository files. None of the files introduced for issues #3-#22 was reported. Running the formatter across the baseline would create an unrelated repository-wide line-ending rewrite. |
| Module tidiness | PASS WITH ENVIRONMENT CAVEAT | `go mod tidy` identified `github.com/microsoft/kiota-http-go` as a direct dependency; commit `1be4145` records that correction. Linux rewrites the tracked CRLF `go.sum`, so the Make target cannot finish cleanly in this checkout without a baseline line-ending change. |
| Module checksum verification | PASS | `go mod verify` completed successfully against the container's module cache. |
| Lint | NOT RUN | `golangci-lint` is absent from the development image. |
| Manifest and annotation checks | PASS VIA FULL SUITE | The extension, server, and tool test packages passed in `make test`, including manifest/schema and conservative aggregate-annotation assertions. The standalone MCPB validator is not installed. |
| MCPB validation | NOT RUN | `mcpb` is absent from the development image. |
| Release configuration and snapshot | NOT RUN | GoReleaser is absent from the development image. |
| SBOM and vulnerability scan | NOT RUN | Syft, Grype, and `govulncheck` are absent from the development image. |
| Licence scan | NOT RUN | Syft and Grant are absent from the development image. |
| CRUD lifecycle | NOT RUN | The harness requires authenticated accounts and real shared-resource fixtures; none were supplied. |

Independent standards and specification review found unsafe mutation retry
behavior, mounted-reselection identity retention, unpinned shared message and
calendar paging, unlocked registry pointers, concurrent alias lost updates,
account label-reuse races, ambiguous calendar and own-mail send outcomes, and
documentation gaps. The corrections now have focused regression coverage,
including exact continuation-link pinning, single-attempt mutation assertions,
durable concurrent alias updates, lifecycle transaction serialization, and
reconciliation guidance for uncertain outcomes. Final independent standards
and specification reviews found no remaining code or governance blocker. The
complete coverage and race suites above pass.

The missing release executables are environment gaps, not passes. Run the
project's normal CI image or install its pinned toolchain before reconsidering
this decision.

## Required live compatibility evidence

The following scenarios from CR-0067 and
`docs/prompts/mcp-tool-crud-test.md` remain mandatory:

1. Personal shared mounted-calendar reads succeed while shared management and
   shared-mail configuration fail locally.
2. Organizational owner-primary and mounted calendar reads use their exact
   owner and recipient views.
3. Organizational mounted non-meeting create, update, reschedule, and delete
   remain on the selected mounted route; attendee-bearing, online, unknown, and
   meeting-specific mutations stop before the write.
4. Organizational shared-mail read, draft, small attachment, move, archive,
   trash, restore, permanent-delete, and confirmed-send lifecycles use the
   immutable owner view and target-bound references.
5. Missing Exchange read/write/send rights produce Microsoft-authoritative
   failures without local privilege escalation or route fallback.
6. Personal, organizational, and unknown token contexts match the published
   matrix; unknown contexts fail closed.
7. Existing own-calendar and own-mail behavior remains backward compatible.
8. Deliberate policy changes between preflight and mutation stop the unstarted
   write or send. Destructive and send operations are never retried
   automatically, and uncertain outcomes are reported as uncertain.
9. Tampered, stale, cross-account, cross-resource, cross-view, and wrong-kind
   references fail locally. A local denial must produce zero Graph traffic.

Any misrouting, silent authority expansion, automatic destructive/send retry,
or incorrectly classified uncertain result is an immediate no-go.

## Large shared attachment boundary

Shared attachments at or above 3 MiB are explicitly unsupported and rejected
locally. The evidence and rationale are recorded in
`docs/reference/shared-large-upload-evidence.md`. Own-mail resumable uploads are
unchanged. No large shared-upload implementation ticket is a prerequisite for
this readiness run because no positive live compatibility evidence exists.

## Rerun requirements

Before issue #22 can close:

1. Run `make ci` in the pinned CI environment, resolving or formally governing
   the repository-wide line-ending baseline.
2. Run `make security`, `make snapshot`, and the MCPB validation target with the
   required pinned tools installed.
3. Execute `make crud-test` with disposable personal and organizational
   accounts, a genuinely delegated owner-primary calendar, an editable mounted
   calendar, a shared mailbox, controlled Exchange rights, and disposable
   messages/events/files.
4. Preserve sanitized route, denial, retry, uncertainty, and compatibility
   evidence for every scenario above.
5. Re-run the standards and specification reviews after any corrections.

Only a complete pass may unblock issue #23. This run does not authorize a pull
request, merge, tag, release, or publication.
