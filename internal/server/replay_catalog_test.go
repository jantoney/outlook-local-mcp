package server

import (
	"testing"

	"github.com/desek/outlook-local-mcp/internal/tools"
	"github.com/mark3labs/mcp-go/mcp"
)

// TestReplayCatalogUsesExplicitVerbAnnotations verifies recovery classification
// is derived from authoritative annotations and fails closed when absent.
func TestReplayCatalogUsesExplicitVerbAnnotations(t *testing.T) {
	catalog := NewReplayCatalog()
	catalog.AddVerbs("calendar", []tools.Verb{
		{Name: "list_events", Annotations: []mcp.ToolOption{mcp.WithReadOnlyHintAnnotation(true)}},
		{Name: "create_event", Annotations: []mcp.ToolOption{mcp.WithReadOnlyHintAnnotation(false)}},
		{Name: "future_verb"},
	})

	if !catalog.Contains("calendar.list_events") {
		t.Fatal("read-only verb missing from replay catalog")
	}
	for _, identity := range []string{"calendar.create_event", "calendar.future_verb", "calendar.unknown"} {
		if catalog.Contains(identity) {
			t.Fatalf("unsafe identity %q unexpectedly replayable", identity)
		}
	}
}
