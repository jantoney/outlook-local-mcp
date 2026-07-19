# Choose the owned Entra application authentication contract

Labels: `wayfinder:research`
Status: closed

## Question

What exact Entra app-registration audience, public-client settings, delegated Graph permissions, tenant authority, and MCP configuration must the fork use to authenticate work, school, and personal Microsoft accounts without relying on borrowed first-party client IDs?

## Evidence to verify

- `signInAudience` must cover organisational directories and personal Microsoft accounts.
- Device-code authentication requires public-client flow to be enabled.
- `common` is the authority for both organisational and personal accounts; `consumers` is personal-only.
- The requested delegated Graph scopes must be valid for personal accounts.
- No client secret belongs in this local public-client application.

## Resolution

Use an owned Entra public native-client registration with:

- `signInAudience: AzureADandPersonalMicrosoftAccount`;
- `requestedAccessTokenVersion: 2`;
- public-client flow enabled (`allowPublicClient: true`);
- no client secret and no application permissions;
- authority/tenant `common`;
- installed-client redirect URIs `http://localhost` and `https://login.microsoftonline.com/common/oauth2/nativeclient`;
- delegated Graph permissions `User.Read`, `Calendars.ReadWrite`, `Mail.Read`, and `Mail.ReadWrite`; and
- no `Mail.Send` permission.

The installed configuration already selects device-code auth and tenant `common`, but it omits `OUTLOOK_MCP_CLIENT_ID` and therefore uses Microsoft Office's first-party client ID. Reliance on that external app registration is the most likely personal-account failure point. Switching to the owned client ID requires a new cache partition or clearing the old authentication record/token cache before fresh consent.

The fork must also add `User.Read` to its requested scopes. It calls Graph `GET /me` to resolve the signed-in address, and that API requires `User.Read`; the current scope builder omits it.

Organisational consent policy or Conditional Access may still block device-code flow. Such a policy failure is distinct from personal-account audience support and must be surfaced through troubleshooting guidance.

## Sources

- <https://learn.microsoft.com/en-us/entra/identity-platform/reference-app-manifest>
- <https://learn.microsoft.com/en-us/entra/identity-platform/msal-client-application-configuration>
- <https://learn.microsoft.com/en-us/entra/identity-platform/msal-client-applications>
- <https://learn.microsoft.com/en-us/graph/permissions-reference>
- <https://learn.microsoft.com/en-us/graph/api/user-get?view=graph-rest-1.0>
- <https://learn.microsoft.com/en-us/troubleshoot/entra/entra-id/app-integration/error-code-aadsts50020-user-account-identity-provider-does-not-exist>
