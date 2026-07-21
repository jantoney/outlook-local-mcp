package server

import (
	"strings"
	"testing"

	"github.com/desek/outlook-local-mcp/internal/auth"
	"github.com/desek/outlook-local-mcp/internal/config"
	"github.com/desek/outlook-local-mcp/internal/graph"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

// TestSharedMailReadSchemaIsStatic verifies discovery exposes shared selectors
// and conditional references without making raw own IDs unconditionally required.
func TestSharedMailReadSchemaIsStatic(t *testing.T) {
	identity := func(handler mcpserver.ToolHandlerFunc) mcpserver.ToolHandlerFunc { return handler }
	verbs, _ := buildMailVerbs(mailVerbsConfig{
		registry: auth.NewAccountRegistry(), retryCfg: graph.RetryConfig{}, cfg: config.Config{},
		authMW: identity, accountResolverMW: identity,
	})
	for _, name := range []string{"list_folders", "list_messages", "get_message"} {
		verb := calendarVerbByName(t, verbs, name)
		tool := mcp.NewTool(name, verb.Schema...)
		if _, ok := tool.InputSchema.Properties["shared_resource"]; !ok {
			t.Fatalf("%s schema missing shared_resource", name)
		}
		if !strings.Contains(verb.Description, "shared") && !strings.Contains(verb.Description, "owner") {
			t.Fatalf("%s help omits shared owner routing: %s", name, verb.Description)
		}
		if name == "list_messages" {
			if _, ok := tool.InputSchema.Properties["folder_ref"]; !ok {
				t.Fatal("list_messages schema missing folder_ref")
			}
		}
		if name == "get_message" {
			if _, ok := tool.InputSchema.Properties["resource_ref"]; !ok {
				t.Fatal("get_message schema missing resource_ref")
			}
			for _, required := range tool.InputSchema.Required {
				if required == "message_id" {
					t.Fatal("get_message message_id remained unconditionally required")
				}
			}
		}
	}
}
