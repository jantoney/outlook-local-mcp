---
name: mail-category-discovery-and-filtering
description: Discover an account's Outlook master categories and filter messages by exact category names.
id: "CR-0072"
status: "implemented"
date: 2026-07-25
requestor: project maintainer
stakeholders: project maintainer, MCP mail users, security reviewers
priority: "medium"
target-version: next minor release
source-branch: codex/shared-outlook-resources
source-commit: b1dc7a9
---

# Mail Category Discovery and Message Filtering

## Change Summary

Add an own-mail `mail.list_categories` verb that reads Outlook's master
category list without scanning messages. Extend `mail.list_messages` with
exact category-name filters supporting either any-category or all-category
matching for own and authorized shared mailboxes.

## Motivation and Background

Users organize mail with Outlook category tags such as `INVOICE - UNPAID`,
`INVOICE PAID`, and `To include in TAX`. The current MCP returns categories in
structured message results but cannot discover the account's configured
category names or ask Graph to select messages by category.

Scanning recent messages is not a substitute for master-category discovery:
unused categories would be missed, historical mail may exceed the tool's result
cap, and repeatedly reading messages consumes unnecessary Graph calls and MCP
response tokens.

Microsoft Graph exposes master categories through
`/me/outlook/masterCategories`. That endpoint requires the delegated
`MailboxSettings.Read` permission. Message categories are a collection property
and can be filtered through OData on the existing messages collection.

## Current State

`mail.list_messages` accepts structured filters for date, sender, conversation,
read state, draft state, attachments, importance, follow-up flag, and MCP
provenance. Its selected fields already include `categories`.

`mail.search_messages` accepts KQL. Microsoft Graph's documented searchable
mail properties do not include Outlook categories, and Graph message `$search`
cannot be combined with the structured `$filter` used by `list_messages`.

The account scope union requests `Mail.Read` for an own-mail read policy, but
does not request `MailboxSettings.Read`.

## Proposed Change

Register `list_categories` as a read-only verb in the existing aggregate
`mail` tool. The verb reads the selected account's own master category
collection and returns:

* text: a numbered list containing display name and color;
* summary: compact objects containing `displayName` and `color`;
* raw: complete category objects containing `id`, `displayName`, and `color`.

Add `categories` and `category_match` parameters to `list_messages`.
`categories` is a comma-separated list of exact Outlook category display names.
`category_match=any` is the default and joins category predicates with `or`.
`category_match=all` joins category predicates with `and`.

```mermaid
flowchart LR
    U[User asks for categories] --> L[mail.list_categories]
    L --> M[/me/outlook/masterCategories]
    M --> N[Names and colors]
    N --> F[mail.list_messages categories filter]
    F --> Q[/messages with OData category predicates]
```

## Requirements

### Functional Requirements

1. The mail aggregate tool **MUST** register a `list_categories` verb.
2. `list_categories` **MUST** require the selected own-mail target's `read`
   capability.
3. `list_categories` **MUST** reject shared-mail selection.
4. `list_categories` **MUST** call the selected account's
   `/me/outlook/masterCategories` route without reading messages.
5. `list_categories` **MUST** accept `text`, `summary`, and `raw` output modes.
6. `list_categories` text output **MUST** list each category's display name and
   preset color.
7. `list_categories` summary output **MUST** include only `displayName` and
   `color`.
8. `list_categories` raw output **MUST** include `id`, `displayName`, and
   `color`.
9. `list_messages` **MUST** accept a comma-separated `categories` parameter.
10. Category names **MUST** be trimmed and empty entries **MUST** be ignored.
11. Category names **MUST** be escaped as OData string literals.
12. `category_match` **MUST** accept only `any` and `all`.
13. An omitted `category_match` **MUST** behave as `any`.
14. `category_match=any` **MUST** return messages containing at least one
    requested category.
15. `category_match=all` **MUST** return messages containing every requested
    category.
16. Category predicates **MUST** combine with existing `list_messages` filters
    using logical `and`.
17. Category filtering **MUST** support both own and authorized shared mailbox
    message routes.
18. Enabling own-mail read capability **MUST** add
    `MailboxSettings.Read` to the account's required delegated scope union.
19. Removing own-mail read capability **MUST** remove
    `MailboxSettings.Read` unless another future capability requires it.
20. Existing aggregate tool annotations **MUST** remain conservative and
    unchanged.
21. The extension manifest **MUST** remain unchanged because no top-level MCP
    tool is added.

### Non-Functional Requirements

1. Category discovery **MUST NOT** enumerate or inspect messages.
2. Category filtering **MUST** occur server-side through Microsoft Graph.
3. Graph errors **MUST** use the existing redaction and timeout behavior.
4. New production functions and types **MUST** have complete Go doc comments.
5. The implementation **MUST** use small single-purpose files.
6. Existing own and shared mail authorization boundaries **MUST** remain intact.

## Affected Components

* `internal/tools/list_categories.go`
* `internal/tools/category_format.go`
* `internal/tools/message_category_filter.go`
* `internal/tools/list_messages.go`
* `internal/server/mail_verbs.go`
* `internal/server/mail_target_guards.go`
* `internal/auth/mail_policy_scopes.go`
* `internal/auth/scope_union.go`
* Mail verb metadata, annotation, schema, and protocol tests
* `docs/concepts.md`
* `docs/prompts/mcp-tool-crud-test.md`

## Scope Boundaries

### In Scope

* Read-only discovery of the signed-in account's master category list.
* Exact category-name filtering on message collections.
* Any-category and all-category matching.
* Category filters combined with dates, sender, folder, and other existing
  `list_messages` filters.
* Required delegated scope calculation and re-consent behavior.
* Own and shared message filtering.

### Out of Scope ("Here, But Not Further")

* Creating, renaming, recoloring, or deleting master categories.
* Assigning or removing categories on messages.
* Reading a shared mailbox owner's master category settings.
* Fuzzy or semantic category-name matching.
* Adding category syntax to KQL `search_messages`.
* Combining Graph message `$search` and `$filter`.
* Introducing a fifth top-level MCP domain.

## Alternative Approaches Considered

* Scanning message results client-side was rejected because it misses unused
  categories and is incomplete beyond result limits.
* Passing `category:` through KQL was rejected because categories are not a
  documented searchable message property.
* Adding only a single `category` parameter was rejected because invoice and
  tax workflows commonly require any-of or all-of selection.
* Enabling shared master-category discovery was rejected because message
  delegation does not itself establish mailbox-settings authority.

## Impact Assessment

### User Impact

Users can first ask what categories exist, then use exact names to retrieve
tagged messages. Accounts with own-mail read enabled require one re-consent for
the additional least-privileged `MailboxSettings.Read` scope.

### Technical Impact

One read-only mail verb and two optional `list_messages` schema properties are
added. Existing message serialization already includes categories, so no
response migration is required. The top-level extension manifest remains
unchanged.

### Business Impact

Category-driven bookkeeping workflows become deterministic and cheaper than
mailbox scanning. The feature does not add hosted infrastructure or write risk.

## Implementation Approach

1. Add `MailboxSettings.Read` to the deterministic own-mail read scope union.
2. Add an own-mail-only read target guard.
3. Implement category collection retrieval and three-tier formatting.
4. Register `list_categories` in the mail verb registry.
5. Parse and build category predicates in a dedicated filter helper.
6. Feed those predicates into `list_messages`.
7. Update registry metadata, scope documentation, and CRUD coverage.

The pinned `microsoftgraph/msgraph-sdk-go` v1.96.0 source confirms the generated
call chain
`UserItemRequestBuilder.Outlook().MasterCategories().Get(ctx, config)` and the
`OutlookCategoryCollectionResponseable` result type. DeepWiki was unavailable,
so validation used the exact locally downloaded module listed in `.deepwiki`.

## Test Strategy

### Tests to Add

| Test File | Test Name | Description | Inputs | Expected Output |
|-----------|-----------|-------------|--------|-----------------|
| `internal/tools/list_categories_test.go` | `TestListCategoriesOutputs` | Three-tier category output | Graph category response | text, summary, and raw contracts |
| `internal/tools/list_categories_test.go` | `TestListCategoriesGraphError` | Redacted Graph failure | Graph 403 | MCP error |
| `internal/tools/message_category_filter_test.go` | `TestBuildCategoryFilter` | Any/all and escaping | category names and mode | exact OData predicate |
| `internal/tools/list_messages_test.go` | `TestListMessagesCategoryFilter` | Handler sends category filter | categories and mode | captured Graph filter |
| `internal/server/mail_shared_read_schema_test.go` | `TestListCategoriesIsOwnOnly` | Shared selector excluded | mail schema and call | local rejection |

### Tests to Modify

| Test File | Test Name | Current Behavior | New Behavior | Reason for Change |
|-----------|-----------|------------------|--------------|-------------------|
| `internal/auth/profile_test.go` | `TestScopesForProfile` | mail scopes only | read profiles include settings scope | least-privileged category discovery |
| `internal/auth/profile_test.go` | `TestScopesForMailPolicy` | read maps to `Mail.Read` | read also maps to settings scope | new API permission |
| `internal/auth/scope_union_test.go` | `TestOAuthScopeUnion` | no settings scope | deterministic settings order | stable scope union |
| `internal/tools/tool_description_test.go` | mail operation coverage | existing verbs | includes `list_categories` | discovery metadata |
| `internal/tools/tool_annotations_test.go` | annotation coverage | existing verbs | includes new read verb | compliance |
| `docs/prompts/mcp-tool-crud-test.md` | mail read steps | no category discovery/filter | exercises both | surface lifecycle coupling |

### Tests to Remove

No tests are removed. No existing capability is deleted or superseded.

## Acceptance Criteria

### AC-1: Discover categories without reading mail

```gherkin
Given an authenticated account with own-mail read enabled
When mail.list_categories is called
Then the server calls only the account's masterCategories endpoint
  And it returns every defined category name and color
  And it does not enumerate messages
```

### AC-2: Filter by one category

```gherkin
Given messages tagged with INVOICE - UNPAID
When mail.list_messages is called with categories set to INVOICE - UNPAID
Then only messages containing that exact category are returned
```

### AC-3: Match any requested category

```gherkin
Given messages tagged with either INVOICE PAID or Added to ZOHO BOOKS
When mail.list_messages is called with both categories and category_match set to any
Then messages containing either category are returned
```

### AC-4: Match every requested category

```gherkin
Given one message contains both INVOICE PAID and To include in TAX
When mail.list_messages is called with both categories and category_match set to all
Then that message is returned
  And a message containing only one requested category is not returned
```

### AC-5: Combine category and date filters

```gherkin
Given categorized messages inside and outside a requested date range
When mail.list_messages is called with categories and date boundaries
Then Graph receives all predicates joined with logical and
  And only categorized messages inside the range are returned
```

### AC-6: Preserve shared-mail authorization

```gherkin
Given an authorized organizational shared mailbox with read capability
When mail.list_messages is called with a category filter and that shared alias
Then the request remains pinned to the configured owner message route
  And raw shared identifiers remain rejected
```

### AC-7: Reject shared category discovery

```gherkin
Given a caller supplies a shared_resource to mail.list_categories
When target authorization runs
Then the call is rejected before a Graph request is constructed
```

### AC-8: Request required consent

```gherkin
Given an account enables own-mail read capability
When its required OAuth scope union is calculated
Then Mail.Read is present
  And MailboxSettings.Read is present
```

## Quality Standards Compliance

### Build and Compilation

- [ ] `make build` passes.
- [ ] `make vet` passes.
- [ ] No compiler warnings are introduced.

### Linting and Code Style

- [ ] `make fmt-check` passes.
- [ ] `make lint` passes.
- [ ] New symbols have required Go documentation.

### Test Execution

- [ ] Focused category and scope tests pass.
- [ ] Existing mail tool tests pass.
- [ ] Full race-enabled repository tests pass.

### Documentation

- [ ] Registry help documents both operations.
- [ ] Concepts document category discovery, filtering, and permission scope.
- [ ] CRUD prompt exercises the changed surface.

### Code Review

- [ ] Changes are submitted through a pull request.
- [ ] PR title follows Conventional Commits.
- [ ] Changes are squash-merged.

### Verification Commands

```bash
make fmt
make build
make vet
make fmt-check
make tidy
make lint
make test
make ci
```

## Risks and Mitigation

### Risk 1: Existing accounts lack the new consent

**Likelihood:** high

**Impact:** medium

**Mitigation:** Add the permission to deterministic required scopes so the
existing policy-change authentication workflow clearly requires re-consent.

### Risk 2: Category names contain OData-sensitive characters

**Likelihood:** low

**Impact:** medium

**Mitigation:** Escape every category value with the existing OData literal
helper and cover apostrophes in unit tests.

### Risk 3: Complex filters trigger Graph InefficientFilter

**Likelihood:** medium

**Impact:** medium

**Mitigation:** Suppress explicit `$orderby` when category collection
predicates are active, matching existing complex-filter handling.

### Risk 4: Shared category discovery is mistaken for message delegation

**Likelihood:** medium

**Impact:** high

**Mitigation:** Keep `list_categories` own-mail only and document the boundary
in schema help and concepts.

## Dependencies

* Microsoft Graph `outlook/masterCategories` v1.0 endpoint.
* Delegated `MailboxSettings.Read`.
* Pinned `microsoftgraph/msgraph-sdk-go` v1.96.0.
* Existing aggregate mail registry and target guard.
* Existing output-tier, retry, timeout, and error-redaction helpers.

## Estimated Effort

One focused implementation session covering governance, code, tests,
documentation, and local quality validation.

## Decision Outcome

Chosen approach: direct own-mail master-category discovery plus server-side
message category filtering, because it is complete, token-efficient, and uses
the least-privileged Graph endpoints for each concern.

## Implementation Status

* **Started:** 2026-07-25
* **Completed:** 2026-07-25
* **Deployed to Production:** not deployed
* **Notes:** Implemented direct own-mail category discovery, any/all exact
  message category filters, least-privileged scope calculation, registry help,
  embedded concepts, CRUD coverage, and focused tests. Formatting, compilation,
  `go vet ./...`, auth tests, server tests, docs tests, and all changed tool
  tests pass. The complete sandbox test run remains blocked only by pre-existing
  attachment-path tests that cannot access Windows temporary paths.

## Related Items

* [CR-0060 domain aggregated tools](CR-0060-domain-aggregated-tools-with-verb-operations.md)
* [CR-0065 registry-driven tool reference](CR-0065-user-documentation-architecture-and-registry-driven-tool-reference.md)
* [CR-0067 shared Outlook resources](CR-0067-per-account-shared-outlook-resources.md)
* [CR-0068 granular mail action policies](CR-0068-granular-mail-action-policies.md)
