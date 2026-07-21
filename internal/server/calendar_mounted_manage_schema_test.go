package server

import (
	"fmt"
	"strings"
	"testing"

	"github.com/desek/outlook-local-mcp/internal/auth"
	"github.com/desek/outlook-local-mcp/internal/graph"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

// TestMountedCalendarManageSchemaDocumentsExactBoundary verifies only the
// non-meeting event verbs expose mounted selection and reference inputs.
func TestMountedCalendarManageSchemaDocumentsExactBoundary(t *testing.T) {
	identity := func(handler mcpserver.ToolHandlerFunc) mcpserver.ToolHandlerFunc { return handler }
	verbs, _ := buildCalendarVerbs(calendarVerbsConfig{
		registry: auth.NewAccountRegistry(), retryCfg: graph.RetryConfig{}, defaultTimezone: "UTC",
		authMW: identity, accountResolverMW: identity,
	})
	for _, name := range []string{"create_event", "update_event", "delete_event", "reschedule_event"} {
		verb := calendarVerbByName(t, verbs, name)
		tool := mcp.NewTool(name, verb.Schema...)
		if _, ok := tool.InputSchema.Properties["shared_resource"]; !ok {
			t.Fatalf("%s schema missing shared_resource", name)
		}
		if !strings.Contains(verb.Description, "mounted") || !strings.Contains(verb.Description, "non-meeting") {
			t.Fatalf("%s help omits mounted non-meeting boundary: %s", name, verb.Description)
		}
		if name != "create_event" {
			if _, ok := tool.InputSchema.Properties["resource_ref"]; !ok {
				t.Fatalf("%s schema missing resource_ref", name)
			}
		}
		if name == "create_event" || name == "update_event" {
			property := tool.InputSchema.Properties["is_online_meeting"]
			description := fmt.Sprint(property)
			if !strings.Contains(description, "own") && !strings.Contains(description, "rejected") {
				t.Fatalf("%s online-meeting help omits mounted exclusion: %s", name, description)
			}
		}
	}
	for _, name := range []string{"respond_event", "cancel_meeting"} {
		tool := mcp.NewTool(name, calendarVerbByName(t, verbs, name).Schema...)
		if _, ok := tool.InputSchema.Properties["shared_resource"]; ok {
			t.Fatalf("%s unexpectedly exposes shared routing", name)
		}
		if _, ok := tool.InputSchema.Properties["resource_ref"]; ok {
			t.Fatalf("%s unexpectedly exposes shared provenance", name)
		}
	}
}
