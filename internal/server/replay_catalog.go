package server

import (
	"sort"

	"github.com/desek/outlook-local-mcp/internal/tools"
	"github.com/mark3labs/mcp-go/mcp"
)

// ReplayCatalog contains exact domain.operation identities that the
// authoritative verb registry marks read-only. Absence always means unsafe to
// replay, which keeps new and unknown operations fail-closed.
type ReplayCatalog map[string]struct{}

// NewReplayCatalog returns an empty replay catalog ready to receive verbs.
// It has no side effects and cannot fail.
func NewReplayCatalog() ReplayCatalog {
	return make(ReplayCatalog)
}

// AddVerbs adds the read-only verbs for domain by evaluating the same MCP
// annotation options used by tool registration. It does not execute handlers.
// Verbs lacking an explicit true readOnlyHint are deliberately omitted.
func (c ReplayCatalog) AddVerbs(domain string, verbs []tools.Verb) {
	for _, verb := range verbs {
		definition := mcp.NewTool(domain+"."+verb.Name, verb.Annotations...)
		if definition.Annotations.ReadOnlyHint != nil && *definition.Annotations.ReadOnlyHint {
			c[domain+"."+verb.Name] = struct{}{}
		}
	}
}

// Contains reports whether identity is explicitly safe to replay. It returns
// false for empty, unknown, and write identities and has no side effects.
func (c ReplayCatalog) Contains(identity string) bool {
	_, ok := c[identity]
	return ok
}

// Sorted returns a deterministic copy of the catalog identities for broker
// metadata and tests. Mutating the returned slice does not affect the catalog.
func (c ReplayCatalog) Sorted() []string {
	identities := make([]string, 0, len(c))
	for identity := range c {
		identities = append(identities, identity)
	}
	sort.Strings(identities)
	return identities
}
