# Choose the own and shared Graph routing seam

Labels: `wayfinder:grilling`
Status: closed
Assignee: /root
Blocked by: `Inventory the shared target SDK seams`, `Choose shared resource identity and ID provenance`

## Question

What small module boundary should handlers depend on so own resources route
through `Me()` and shared resources route through `Users().ByUserId(owner)`,
while preserving typed SDK usage, cancellation, retries, response tiering, and
testability across the supported operation matrix?

## Working decisions

- The Graph routing module has one responsibility: accept the selected Graph
  client and an already-resolved resource target, then return the SDK's typed
  `*users.UserItemRequestBuilder` according to mailbox view. Recipient-view
  targets use `client.Me()`; owner-view targets use
  `client.Users().ByUserId(owner)`.
- The routing module does not resolve aliases, authorize capabilities, validate
  signed references, construct raw URLs, apply retries or timeouts, serialize
  responses, or mirror downstream calendar and mail builders. Existing handlers
  retain their typed SDK chains and operation-specific behavior.
- Target-resolution middleware runs immediately after account resolution,
  resolves the account-scoped `shared_resource` selector, and places one
  immutable resolved resource target in request context. Handlers consume that
  target and must not parse, look up, or reinterpret the selector themselves.
- Own resources and mounted calendars are recipient-view targets and route
  through `Me()`. Shared mailboxes and owner-primary calendars are owner-view
  targets and route through `Users().ByUserId(owner)`. Routing never infers the
  SDK root from resource ownership alone.
- Target-resolution middleware resolves the alias, enforces token-tenant and
  resource-kind compatibility, and checks the operation's exact target
  capability as one module operation. It places a resolved resource target in
  context only after every check passes; unauthorized intermediate targets are
  not observable by handlers.
- Signed resource-reference verification remains a separate operation-level
  module because it depends on each verb's message, folder, event, draft, or
  attachment arguments. Verification must complete before the handler builds a
  Graph item route.
- A new `internal/resource` package owns the resolved-target value, context
  helpers, account-scoped alias resolution, mailbox-view classification,
  account compatibility, and exact capability authorization. `internal/auth`
  continues to own signed-in account selection, while `internal/graph` owns
  only conversion of an authorized resource target into the typed SDK user
  root. Neither auth nor Graph absorbs the complete target-policy model.
- The resolved resource target is an immutable value snapshot created once per
  request. It contains no account-registry pointers, mutable policy maps, Graph
  clients, or prebuilt SDK request builders. Policy changes affect subsequent
  requests without mutating an in-flight target; the selected Graph client
  remains a separate request-context dependency.
- The target combines stable provenance identity with validated routing facts:
  immutable signed-in account ID; an exact resource-kind discriminant;
  immutable shared-resource ID where applicable; mailbox view; presentation-
  only alias, display, and masked owner snapshots; a validated owner route
  locator only for owner-view targets; and a mounted calendar ID only for
  mounted-calendar targets. It excludes raw request selectors and the complete
  capability policy.
- Constructors and unexported fields enforce valid combinations. Own targets
  carry no shared identity, alias, owner route, or mounted ID. Owner-view shared
  targets carry shared identity and owner route but no mounted ID. Mounted
  targets carry shared identity and mounted ID in recipient view; their owner
  snapshot is audit evidence and can never select a Graph route.
- Alias and display snapshots may become stale during an in-flight request and
  are used only for human output and audit history. Immutable account/resource
  identities and mailbox view remain authoritative for routing, authorization,
  revocation, and signed-reference validation.
- Authorization lasts for one independently committable stage, not for an
  arbitrary TTL or an entire long-running handler. Revalidate current account,
  immutable resource identity, exact capability, signed reference, and route
  equality before each later stage. Revalidation uses immutable IDs, never the
  original alias text.
- Once a bounded unit starts, required invariant-completion work keeps its
  original immutable target. Draft creation plus required provenance marking is
  one unit; a marking failure reports visible partial success rather than
  silently deleting the draft. Attachment upload revalidates before session
  creation and every chunk, with the final chunk plus narrow verification as a
  terminal unit. Post-elicitation send rebuilds and reauthorizes the target,
  revalidates the reference and reviewed draft snapshot, and checks authority
  immediately before the single non-retried send call.
- Account or resource removal stops every unstarted stage. Alias rename does
  not retarget non-human work, but a changed presentation snapshot invalidates
  outstanding human confirmation. Failures identify whether nothing started,
  partial work exists, upload stopped unverified, confirmation became stale, or
  send outcome is uncertain.
- Testing uses a narrow hybrid rather than a project-owned interface that
  mirrors the Graph SDK. `internal/resource` receives table-driven and fuzz
  tests through validated constructors and small repository/policy fakes.
  `internal/graph` and representative handlers use the real generated SDK over
  the existing `httptest` transport to assert exact `/me` versus
  `/users/{owner}` paths, request methods, coupled-call ordering, cancellation,
  retry counts, response tiers, and zero requests on local denial.
- Resolved-target fields are unexported and invalid combinations are rejected
  by constructors. Because Go still permits a zero value, the SDK root selector
  returns an error for an invalid target rather than treating it as `/me`.
  Transport scripts cover throttling, unavailable service, timeout,
  cancellation, malformed responses, connection failure, and non-retried
  ambiguous mutations while keeping every retry on the identical target route.
- The middleware order is `auth -> observability -> AuditAttemptWrap ->
  AccountResolver -> ReadOnlyGuard -> ResourceResolveAuthorize ->
  ReferenceGuard (when required) -> handler`. Read-only denial intentionally
  avoids alias resolution; resource resolution and authorization use one
  consistent registry snapshot and occur exactly once.
- `AuditAttemptWrap` creates a concurrency-safe, non-authoritative, write-once
  evidence recorder and finalizes exactly one event. Each phase may append an
  immutable fact snapshot once but cannot overwrite prior facts. Authorization,
  routing, and handlers never read the recorder. It captures structured phase,
  outcome, reason code, and phase-appropriate sanitized facts for account,
  read-only, target-resolution, reference, and handler outcomes.
- Failed resolution produces no partial or fake target. Unknown aliases record
  only sanitized selector evidence; compatibility and capability denials may
  record stable candidate identity and kind facts without owner route details.
  Successful resolution records authoritative target identity/view snapshots.
  Missing target inside a handler is an internal invariant failure and never
  falls back to `/me`.
- Cancellation propagates through every gate and handler. Audit finalization may
  use a short bounded local context solely to preserve cancellation evidence and
  performs no Graph action. Audit failure after a mutation does not change or
  retry the mutation outcome; it is surfaced as a separate operational failure.

## Resolution

Introduce two deliberately narrow modules. `internal/resource` resolves and
authorizes one immutable resource target per request. `internal/graph` exposes
only this typed SDK-root selector:

```go
func UserRoot(
    client *msgraphsdk.GraphServiceClient,
    target resource.ResolvedTarget,
) (*users.UserItemRequestBuilder, error)
```

`UserRoot` accepts only an already-authorized target. Recipient-view own
resources and mounted calendars return `client.Me()`. Owner-view shared
mailboxes and owner-primary calendars return
`client.Users().ByUserId(target.OwnerLocator())`. Mounted-calendar handlers
then apply the validated mounted calendar ID through their existing typed
calendar-builder branch.

The selector rejects nil clients, zero targets, unsupported kinds, kind/view
mismatches, missing owner locators, missing mounted IDs, and every other invalid
combination before Graph traffic. It never falls back to `Me()`. It accepts no
raw owner, alias, request arguments, policy object, Graph URL, or prebuilt
request builder and performs no lookup, authorization, reference validation,
Graph request, retry, timeout, serialization, logging, or audit mutation.

Target middleware runs after account resolution and produces context state only
after alias resolution, tenant compatibility, resource-kind compatibility, and
exact capability authorization all pass. Operation-specific signed-reference
guards run afterward. An outer write-once audit-attempt recorder captures one
structured result across all phases without becoming an authority source.
Authorization is revalidated at independently committable boundaries while
required completion work for an already-started bounded unit retains its
original immutable target.

Handlers retain their generated calendar, mail-folder, message, draft,
attachment, and action builders plus cancellation, retries, timeouts, coupled
call sequencing, response tiering, and write confirmations. Tests use direct
resource-module tests with narrow fakes and the real generated SDK over the
existing local HTTP transport; no broad Graph facade is introduced.

The following exclusions are normative:

- `internal/auth/email_resolver.go` remains a direct `client.Me()` call because
  it resolves the signed-in principal rather than a resource target.
- Shared `calendar.respond_event` and `calendar.cancel_meeting` remain
  unsupported, and shared event deletion must not become an implicit meeting
  cancellation route.
- Mechanical replacement of every `Me()` call is prohibited. Only the approved
  target-aware resource handlers use `UserRoot`.
- Invalid targets and missing target context are internal invariant failures,
  produce zero Graph requests, and never silently select an own resource.

Acceptance coverage proves exact `/me` and `/users/{owner}` paths, mounted
calendar paths under `/me/calendars/{mounted-id}`, zero traffic for every local
denial or invalid target, inability of excluded actions to acquire a shared
route, continued signed-in identity resolution through `/me`, and unchanged
handler behavior beyond the selected root path.
