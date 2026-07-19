# Choose the upstream baseline and compatibility target

Labels: `wayfinder:grilling`
Status: closed

## Question

Should the fork extend current upstream `main` with its aggregate `mail.add_attachment` verb and improved multi-account model, or preserve compatibility with the installed v0.4.0 release surface before upgrading?

## Resolution

Build from `jantoney/outlook-local-mcp` `main`. At decision time fork `main`,
upstream `main`, and the local checkout all resolved to commit `5b22fb5`.
Compatibility targets current `main`; the installed v0.4.0 tool surface is not
preserved when it conflicts with current aggregate-domain conventions.
