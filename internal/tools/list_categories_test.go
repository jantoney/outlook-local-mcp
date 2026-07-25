package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/desek/outlook-local-mcp/internal/auth"
	"github.com/desek/outlook-local-mcp/internal/graph"
	"github.com/mark3labs/mcp-go/mcp"
)

// TestListCategoriesOutputs verifies master categories are fetched directly
// and rendered through all three output tiers.
func TestListCategoriesOutputs(t *testing.T) {
	t.Parallel()

	var requestedPath string
	client, server := newTestGraphClient(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requestedPath = request.URL.Path
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"value":[
			{"id":"cat-1","displayName":"INVOICE - UNPAID","color":"preset1"},
			{"id":"cat-2","displayName":"To include in TAX","color":"preset8"}
		]}`))
	}))
	defer server.Close()

	handler := NewHandleListCategories(graph.RetryConfig{}, time.Second)
	ctx := auth.WithGraphClient(context.Background(), client)

	t.Run("text", func(t *testing.T) {
		result := callListCategories(t, handler, ctx, "text")
		text := result.Content[0].(mcp.TextContent).Text
		if !strings.Contains(text, "INVOICE - UNPAID (preset1)") ||
			!strings.Contains(text, "Total: 2 categories") {
			t.Fatalf("text output = %q", text)
		}
	})

	t.Run("summary", func(t *testing.T) {
		result := callListCategories(t, handler, ctx, "summary")
		var categories []map[string]any
		if err := json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &categories); err != nil {
			t.Fatalf("decode summary: %v", err)
		}
		if len(categories) != 2 || categories[0]["displayName"] != "INVOICE - UNPAID" {
			t.Fatalf("summary categories = %#v", categories)
		}
		if _, exists := categories[0]["id"]; exists {
			t.Fatalf("summary unexpectedly includes id: %#v", categories[0])
		}
	})

	t.Run("raw", func(t *testing.T) {
		result := callListCategories(t, handler, ctx, "raw")
		var categories []map[string]any
		if err := json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &categories); err != nil {
			t.Fatalf("decode raw: %v", err)
		}
		if len(categories) != 2 || categories[0]["id"] != "cat-1" || categories[0]["color"] != "preset1" {
			t.Fatalf("raw categories = %#v", categories)
		}
	})

	if requestedPath != "/v1.0/users/me-token-to-replace/outlook/masterCategories" {
		t.Fatalf("requested path = %q, want SDK own-user masterCategories route", requestedPath)
	}
}

// TestListCategoriesGraphError verifies Graph failures are returned as MCP tool
// errors rather than transport errors.
func TestListCategoriesGraphError(t *testing.T) {
	t.Parallel()

	client, server := newTestGraphClient(t, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusForbidden)
		_, _ = writer.Write([]byte(`{"error":{"code":"ErrorAccessDenied","message":"denied"}}`))
	}))
	defer server.Close()

	handler := NewHandleListCategories(graph.RetryConfig{}, time.Second)
	result, err := handler(auth.WithGraphClient(context.Background(), client), mcp.CallToolRequest{})
	if err != nil {
		t.Fatalf("handler error = %v", err)
	}
	if !result.IsError {
		t.Fatal("expected Graph failure to return an MCP tool error")
	}
}

// callListCategories invokes the category handler with one output mode and
// fails the test on any handler or MCP error.
func callListCategories(
	t *testing.T,
	handler func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error),
	ctx context.Context,
	output string,
) *mcp.CallToolResult {
	t.Helper()
	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]any{"output": output}
	result, err := handler(ctx, request)
	if err != nil {
		t.Fatalf("handler error = %v", err)
	}
	if result.IsError {
		t.Fatalf("tool error = %#v", result.Content)
	}
	return result
}
