# Choose the MCP local-file trust boundary

Labels: `wayfinder:grilling`
Status: closed

## Question

What local paths may the MCP read for attachment upload, what maximum file size and file-type rules apply, and should the operation accept a filesystem path, base64 content, or both?

## Resolution

Accept one filesystem path per invocation. Local upload is disabled until
`OUTLOOK_MCP_ATTACHMENT_ROOTS` explicitly allowlists one or more directories.
Canonical path validation must reject traversal and link escapes outside those
roots. The initial implementation accepts all file types, applies the Graph
150 MiB ceiling, and does not accept inline base64 input.
