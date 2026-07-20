# Fix account bootstrap authentication ordering

Labels: `wayfinder:task`
Status: open
Blocked by: none

## Question

Change aggregate account-verb middleware so bootstrap and recovery operations
such as `account.add`, `account.list`, and `account.login` can execute when the
implicit default account is disconnected, and so the selected account's
requested or persisted authentication method reaches its handler instead of
being pre-empted by default-account authentication.

The correction must preserve observability and audit wrapping, define which
account operations genuinely require an authenticated Graph client, add an
integration regression proving `account.add(auth_method="device_code")`
surfaces a device code while the default account is disconnected and globally
configured for `auth_code`, and update public troubleshooting guidance.
