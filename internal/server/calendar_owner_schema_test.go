package server

import (
	"strings"
	"testing"

	"github.com/desek/outlook-local-mcp/internal/auth"
	"github.com/desek/outlook-local-mcp/internal/graph"
	"github.com/desek/outlook-local-mcp/internal/tools"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

// TestOwnerCalendarReadSchemaIsStatic verifies zero configured aliases do not
// hide shared_resource or resource_ref from discovery and shared get keeps the
// own-only event_id condition out of JSON Schema's unconditional required set.
func TestOwnerCalendarReadSchemaIsStatic(t *testing.T) {
	registry := auth.NewAccountRegistry()
	identity := func(handler mcpserver.ToolHandlerFunc) mcpserver.ToolHandlerFunc { return handler }
	verbs, _ := buildCalendarVerbs(calendarVerbsConfig{
		registry: registry, retryCfg: graph.RetryConfig{}, defaultTimezone: "UTC",
		authMW: identity, accountResolverMW: identity,
	})
	for _, name := range []string{"list_events", "get_event", "search_events"} {
		verb := calendarVerbByName(t, verbs, name)
		tool := mcp.NewTool(name, verb.Schema...)
		if _, ok := tool.InputSchema.Properties["shared_resource"]; !ok {
			t.Fatalf("%s schema missing shared_resource", name)
		}
		if !strings.Contains(verb.Description, "resource_ref") {
			t.Fatalf("%s help description missing reference semantics", name)
		}
		if name != "get_event" {
			continue
		}
		if _, ok := tool.InputSchema.Properties["resource_ref"]; !ok {
			t.Fatal("get_event schema missing resource_ref")
		}
		for _, required := range tool.InputSchema.Required {
			if required == "event_id" {
				t.Fatal("get_event event_id remained unconditionally required")
			}
		}
	}
}

// calendarVerbByName returns one constructed calendar verb for schema tests.
func calendarVerbByName(t *testing.T, verbs []tools.Verb, name string) tools.Verb {
	t.Helper()
	for _, verb := range verbs {
		if verb.Name == name {
			return verb
		}
	}
	t.Fatalf("calendar verb %q not found", name)
	return tools.Verb{}
}
