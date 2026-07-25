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
	for _, name := range []string{"list_folders", "list_messages", "get_message", "search_messages", "get_conversation", "list_attachments", "get_attachment"} {
		verb := calendarVerbByName(t, verbs, name)
		tool := mcp.NewTool(name, verb.Schema...)
		if _, ok := tool.InputSchema.Properties["shared_resource"]; !ok {
			t.Fatalf("%s schema missing shared_resource", name)
		}
		if !strings.Contains(verb.Description, "shared") && !strings.Contains(verb.Description, "owner") {
			t.Fatalf("%s help omits shared owner routing: %s", name, verb.Description)
		}
		if name == "list_messages" || name == "search_messages" {
			if _, ok := tool.InputSchema.Properties["folder_ref"]; !ok {
				t.Fatalf("%s schema missing folder_ref", name)
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
		if name == "get_conversation" || name == "list_attachments" || name == "get_attachment" {
			if _, ok := tool.InputSchema.Properties["message_ref"]; !ok {
				t.Fatalf("%s schema missing message_ref", name)
			}
		}
		if name == "get_attachment" {
			if _, ok := tool.InputSchema.Properties["attachment_ref"]; !ok {
				t.Fatal("get_attachment schema missing attachment_ref")
			}
		}
	}
}

// TestMailCategorySchemas verifies master-category discovery remains own-only
// while message category filters remain available on shared reads.
func TestMailCategorySchemas(t *testing.T) {
	identity := func(handler mcpserver.ToolHandlerFunc) mcpserver.ToolHandlerFunc { return handler }
	verbs, _ := buildMailVerbs(mailVerbsConfig{
		registry: auth.NewAccountRegistry(), retryCfg: graph.RetryConfig{}, cfg: config.Config{},
		authMW: identity, accountResolverMW: identity,
	})

	listCategories := mcp.NewTool("list_categories", calendarVerbByName(t, verbs, "list_categories").Schema...)
	if _, ok := listCategories.InputSchema.Properties["shared_resource"]; ok {
		t.Fatal("list_categories must not advertise shared_resource")
	}

	listMessages := mcp.NewTool("list_messages", calendarVerbByName(t, verbs, "list_messages").Schema...)
	for _, name := range []string{"categories", "category_match"} {
		if _, ok := listMessages.InputSchema.Properties[name]; !ok {
			t.Fatalf("list_messages schema missing %s", name)
		}
	}
}
