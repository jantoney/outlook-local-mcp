package server

import (
	"testing"

	"github.com/desek/outlook-local-mcp/internal/config"
	"github.com/desek/outlook-local-mcp/internal/tools"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

// TestAccountMailAliasVerbContract verifies the aggregate account registry
// exposes every issue #6 lifecycle operation with the expected policy schema.
func TestAccountMailAliasVerbContract(t *testing.T) {
	identity := func(handler mcpserver.ToolHandlerFunc) mcpserver.ToolHandlerFunc { return handler }
	verbs, _ := buildAccountVerbs(accountVerbsConfig{cfg: config.Config{}, authMW: identity})
	byName := make(map[string]tools.Verb, len(verbs))
	for _, verb := range verbs {
		byName[verb.Name] = verb
	}
	for _, name := range []string{"add_mail_alias", "list_mail_aliases", "rename_mail_alias", "remove_mail_alias", "set_mail_alias_policy"} {
		if _, ok := byName[name]; !ok {
			t.Fatalf("account verb %q is not registered", name)
		}
	}
	// label + alias plus the eight independent action switches.
	if got := len(byName["set_mail_alias_policy"].Schema); got != 10 {
		t.Fatalf("set_mail_alias_policy schema options = %d, want 10", got)
	}
}
