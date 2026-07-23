// Package accountadmin owns transport-neutral account lifecycle, permissions,
// and authentication sessions. MCP and HTTP adapters translate requests into
// this package so persistence, token cleanup, and fail-closed state transitions
// have one implementation.
package accountadmin
