# Choose the shared resource rollout and verification plan

Labels: `wayfinder:grilling`
Status: closed
Assignee: /root
Blocked by: `Choose shared resource identity and ID provenance`, `Choose shared capability and account compatibility rules`, `Choose the own and shared Graph routing seam`, `Choose the shared send safety contract`

## Question

In what implementation order should persistence, authentication, account verbs,
calendar routes, mail routes, attachments, sending, documentation, and live
tenant verification land so each checkpoint is backward compatible and the
approved CR can be verified without unsafe shared-mail side effects?

## Resolution

Implement the shared-resource work as a sequence of green, single-purpose
checkpoints. A checkpoint may expose public behavior only when its registry
help, manifest, embedded concepts or troubleshooting guidance where relevant,
CRUD prompt, and contract tests describe that same behavior. Internal-only
checkpoints do not need artificial user documentation. Every checkpoint must
pass its focused quality checks; the completed branch must pass `make ci`, the
CRUD harness, manifest validation, and a release snapshot before push or
release. Release notes remain owned by release-please.

The binding order is:

1. Reconcile CR-0067 and CR-0068 with the completed Wayfinder decisions and
   add legacy migration and secure-default fixtures.
2. Add persisted immutable account/resource identities, the signing-key
   lifecycle, signed-reference primitives, and deterministic legacy mail-policy
   migration. Do not request shared scopes or expose shared routes yet.
3. Add token-tenant classification, exact scope-union calculation, account
   status, and reauthentication only when that union changes. Shared operations
   remain disabled.
4. Add discovery/select/remove and the two explicit calendar/mail alias kinds,
   then issue signed references for selected targets.
5. Add the resolved-resource target, authorization/reference middleware, typed
   `Me()` versus `Users(owner)` user-root seam, deny-path tests, and real-SDK URL
   tests before migrating handlers.
6. Preserve own-calendar regressions, then add owner-primary and mounted
   calendar reads.
7. Add organizational mounted-calendar writes, excluding shared
   `respond_event` and `cancel_meeting`.
8. Add shared-mail reads, folders, and provenance-bearing references.
9. Add shared drafts and small/direct attachments. Add large upload-session
   attachments only after their separate live compatibility gate passes.
10. Add reversible mail mutations in the order move, Archive, trash, and
    restore, with semantic destination guards.
11. Add permanent deletion last among mail mutations. It remains disabled by
    default and requires fresh single-use human elicitation for every message,
    immediate capability/reference revalidation, exactly one Graph request,
    no batching or retry, and an uncertain state for ambiguous post-dispatch
    outcomes.
12. Add shared-send review and confirmation, followed by exactly one isolated
    live send under the approved send-safety contract.
13. Consolidate integration coverage, run the full CRUD lifecycle and release
    checks, build a snapshot, and release through the existing PR and
    release-please process. This stage is not a backlog for deferred public
    contracts.

Automated release gates cover migration and default-off behavior, exact scope
unions, reauthentication, signing-key persistence and reference tamper/cross-
target rejection, every local capability and account-type denial before Graph,
exact route selection with no fallback, read-only enforcement, response tiers,
masking and audit outcomes, confirmation cancellation/staleness, one-attempt
send and permanent-delete behavior, and manifest/help/CRUD/snapshot consistency.

Live release gates cover every scenario claimed at launch: personal shared-
calendar read with manage absent, organizational owner-primary and mounted
calendar reads, organizational mounted non-meeting writes, shared-mail read and
draft lifecycle with a small attachment, a missing-Exchange-right negative
case, one isolated shared send using either available sender right, and one
permanent deletion of a disposable test message after temporary explicit
enablement and fresh confirmation. Live tests use dedicated mailboxes,
calendars, messages, and recipients; they do not intentionally manufacture an
ambiguous send.

Large shared-mail upload sessions, the untested alternative between Send As
and Send on Behalf, tenant-specific Sent Items behavior, and uncommon Exchange
right combinations are evidence-limited. Missing or negative evidence narrows
or gates the advertised support matrix and is documented; it never weakens a
local guard. Misrouting, a denied action reaching Graph, silent capability
enablement, an automatic retry of send or permanent deletion, or an uncertain
result labelled success/failure halts the rollout.

Migration preserves existing own-resource behavior, enables no alias or
`.Shared` scope by default, and never converts a historic calendar ID into a
mounted alias. Users discover and explicitly select mounted resources. Signing
key rotation visibly invalidates old references, which callers must replace by
resolving their stable workflow selectors again.
