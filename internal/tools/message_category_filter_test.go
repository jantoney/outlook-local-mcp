package tools

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/desek/outlook-local-mcp/internal/auth"
	"github.com/desek/outlook-local-mcp/internal/graph"
	"github.com/mark3labs/mcp-go/mcp"
)

// TestParseMessageCategoryFilter verifies trimming, default matching, and
// validation for list_messages category arguments.
func TestParseMessageCategoryFilter(t *testing.T) {
	t.Parallel()

	names, match, err := parseMessageCategoryFilter(" INVOICE PAID, To include in TAX ", "")
	if err != nil {
		t.Fatalf("parseMessageCategoryFilter() error = %v", err)
	}
	if match != "any" || len(names) != 2 || names[0] != "INVOICE PAID" || names[1] != "To include in TAX" {
		t.Fatalf("parseMessageCategoryFilter() = %v, %q, want two names and any", names, match)
	}

	if _, _, err = parseMessageCategoryFilter("", "all"); err == nil {
		t.Fatal("expected category_match without categories to fail")
	}
	if _, _, err = parseMessageCategoryFilter("INVOICE PAID", "some"); err == nil {
		t.Fatal("expected unsupported category_match to fail")
	}
}

// TestBuildMessageCategoryFilter verifies any/all composition and OData
// escaping for exact Outlook category display names.
func TestBuildMessageCategoryFilter(t *testing.T) {
	t.Parallel()

	anyFilter := buildMessageCategoryFilter([]string{"INVOICE PAID", "Director's review"}, "any")
	anyWant := "(categories/any(c0:c0 eq 'INVOICE PAID') or categories/any(c1:c1 eq 'Director''s review'))"
	if anyFilter != anyWant {
		t.Fatalf("any filter = %q, want %q", anyFilter, anyWant)
	}

	allFilter := buildMessageCategoryFilter([]string{"INVOICE PAID", "To include in TAX"}, "all")
	allWant := "(categories/any(c0:c0 eq 'INVOICE PAID') and categories/any(c1:c1 eq 'To include in TAX'))"
	if allFilter != allWant {
		t.Fatalf("all filter = %q, want %q", allFilter, allWant)
	}

	if filter := buildMessageCategoryFilter(nil, "any"); filter != "" {
		t.Fatalf("empty filter = %q, want empty", filter)
	}
}

// TestMessageCategoryFilterSuppressesOrderBy verifies category collection
// predicates use the existing complex-filter path that omits explicit orderby.
func TestMessageCategoryFilterSuppressesOrderBy(t *testing.T) {
	t.Parallel()

	if !filterRequiresNoOrderby(messageFilterOptions{categories: []string{"INVOICE PAID"}}) {
		t.Fatal("category filter should suppress explicit orderby")
	}
}

// TestListMessagesCategoryFilterRequest verifies the handler sends category
// predicates to Graph and omits explicit ordering for the complex filter.
func TestListMessagesCategoryFilterRequest(t *testing.T) {
	t.Parallel()

	var filter, orderby string
	client, server := newTestGraphClient(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		filter = request.URL.Query().Get("$filter")
		orderby = request.URL.Query().Get("$orderby")
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"value":[]}`))
	}))
	defer server.Close()

	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]any{
		"categories":     "INVOICE - UNPAID,To include in TAX",
		"category_match": "all",
	}
	handler := NewHandleListMessages(graph.RetryConfig{}, time.Second, "")
	result, err := handler(auth.WithGraphClient(context.Background(), client), request)
	if err != nil {
		t.Fatalf("handler error = %v", err)
	}
	if result.IsError {
		t.Fatalf("tool error = %#v", result.Content)
	}
	want := "(categories/any(c0:c0 eq 'INVOICE - UNPAID') and categories/any(c1:c1 eq 'To include in TAX'))"
	if filter != want {
		t.Fatalf("$filter = %q, want %q", filter, want)
	}
	if orderby != "" {
		t.Fatalf("$orderby = %q, want omitted", orderby)
	}
}
