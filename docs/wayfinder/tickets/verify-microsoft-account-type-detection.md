# Verify Microsoft account-type detection

Labels: `wayfinder:research`
Status: closed
Assignee: /root/research_auth_contract

## Question

What authoritative signal available through the current Azure Identity/MSAL
authentication record or Microsoft Graph user response can distinguish a
personal Microsoft account from a work/school account without parsing email
shapes or making tenant-name assumptions, and how stable is that signal across
`common`, `organizations`, and `consumers` authority flows?

## Evidence to verify

- Microsoft identity platform documentation for the consumer tenant and account types.
- Fields exposed by the installed Azure Identity authentication records.
- Whether Graph `/me` exposes a reliable account-type discriminator.
- Behavior for guest accounts and organizational accounts authenticated through `common`.
- A safe fallback when account type cannot be established authoritatively.

## Resolution

Use the resolved token tenant as the authoritative discriminator for a direct
personal Microsoft account context. The Microsoft consumer tenant ID is the
fixed GUID `9188040d-6c67-4c5b-b112-36a304b66dad`. Therefore:

- `tenant_id == 9188040d-6c67-4c5b-b112-36a304b66dad` means the token was
  issued in the personal Microsoft account tenant and can be classified as
  `personal`.
- Any other validated tenant GUID means the token was issued in an
  organizational tenant. This establishes an `organizational` token context,
  but does not by itself prove that the user's home identity is a work/school
  account because a personal Microsoft account can be invited into an
  organizational tenant as a guest.
- A missing, malformed, or otherwise unavailable tenant ID produces
  `unknown`; it must not be inferred from an email address, UPN suffix, tenant
  display name, or the formatting of `HomeAccountID`.

This signal is available in the current implementation. Azure Identity
v1.13.1's `AuthenticationRecord.TenantID` is copied from the validated ID
token's `tid` claim (falling back to the issuer path when the claim is absent).
The record exposes only `Authority`, `ClientID`, `HomeAccountID`, `TenantID`,
`Username`, and `Version`. Its `Authority` contains only the issuer scheme and
host, so it does not retain whether the original authority was `common`,
`organizations`, or `consumers`. It also does not retain the ID token's `idp`
or optional `acct` claim. `HomeAccountID` is an opaque account identifier for
this decision even though the installed MSAL code currently constructs it from
client-info fields; its string layout is not the account-type contract.

The manual MSAL path has the same usable signal: MSAL Go's account `Realm` is
the authenticated tenant and is already persisted by this repository. MSAL's
authentication result exposes the parsed ID token during acquisition, but the
current repository discards that token and persists only account fields. The
current persisted data therefore cannot recover `idp` later.

Authority behavior is stable as follows:

- `consumers` accepts only personal Microsoft accounts and resolves to the
  consumer tenant GUID.
- `organizations` accepts only work/school accounts and resolves to an
  organizational tenant GUID.
- `common` accepts both. A direct personal sign-in resolves to the consumer
  tenant GUID; a work/school sign-in resolves to that account's organizational
  tenant GUID. The literal string `common` is not the post-authentication
  discriminator.
- A tenant-specific authority signs the user into that resource tenant and can
  admit a personal account that is a guest there. Its `tid` is the resource
  tenant, not the consumer tenant.

Microsoft documents `idp` as the ID-token claim that records the identity
provider when it differs from the issuer. For a personal account used as an
organizational guest, it may be `live.com` or an STS URI containing the
consumer tenant ID. The optional `acct` claim distinguishes tenant member
(`0`) from guest (`1`). Those claims could support a future, richer distinction
between token context and home identity, but neither is present in the current
Azure Identity authentication record. `idp` is also not guaranteed in every
scenario, so its absence must still yield unknown home origin.

Microsoft Graph `/me` does not supply a universal personal-versus-work/school
field. The selectable `userType` property distinguishes only `Member` from
`Guest`; it describes the user's relationship to the current Entra directory,
not the type of the home identity. Profile values such as `mail`,
`userPrincipalName`, and `identities` must not replace the token-tenant signal.

The required fallback is consequently tri-state:

1. Return `personal` only for the exact consumer tenant GUID.
2. Return `organizational` for another validated tenant GUID when only the
   current token context matters.
3. Return `unknown` for missing/invalid data, and for home-account-type
   questions in a guest organizational context unless a validated ID token's
   `idp` is deliberately retained and establishes the origin.

Unknown must never silently become work/school. If account type gates a
capability, use the conservative feature intersection or let Microsoft Graph
return its supported/unsupported result; if it is display metadata, show
`unknown` explicitly.

## Sources

- [Microsoft identity platform ID token claims reference](https://learn.microsoft.com/en-us/entra/identity-platform/id-token-claims-reference) — defines `tid`, the fixed consumer tenant GUID, and `idp` behavior for personal-account guests.
- [OpenID Connect on the Microsoft identity platform](https://learn.microsoft.com/en-us/entra/identity-platform/v2-protocols-oidc) — defines the accepted account populations for `common`, `organizations`, `consumers`, and tenant-specific authorities.
- [Microsoft identity platform optional claims reference](https://learn.microsoft.com/en-us/entra/identity-platform/optional-claims-reference) — defines `acct` as member (`0`) versus guest (`1`), not personal versus work/school.
- [Microsoft Graph user resource](https://learn.microsoft.com/en-us/graph/api/resources/user?view=graph-rest-1.0) — defines `userType` solely as `Member` or `Guest`.
- [Microsoft Graph get user (`/me`)](https://learn.microsoft.com/en-us/graph/api/user-get?view=graph-rest-1.0) — documents the same `/me` operation for delegated work/school and personal accounts and its returned user resource.
- Installed Azure Identity v1.13.1, `authentication_record.go` — defines the record's exposed fields and copies `TenantID` from the ID token in `newAuthenticationRecord`.
- Installed MSAL Go v1.6.0, `apps/internal/oauth/ops/accesstokens/tokens.go` and `apps/internal/shared/shared.go` — defines the parsed ID-token/account fields and current `HomeAccountID` construction.
