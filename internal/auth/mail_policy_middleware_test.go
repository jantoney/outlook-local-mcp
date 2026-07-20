package auth

import (
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

// TestRequireMailCapabilityUsesExactAction verifies draft authority does not
// imply send authority even when both actions need a broad Graph write scope.
func TestRequireMailCapabilityUsesExactAction(t *testing.T) {
	t.Parallel()

	called := false
	next := func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		called = true
		return mcp.NewToolResultText("called"), nil
	}
	ctx := WithAccountInfo(context.Background(), AccountInfo{
		Label: "operations",
		MailPolicy: MailActionPolicy{
			Read:  true,
			Draft: true,
		},
	})

	result, err := RequireMailCapability(MailCapabilitySend, next)(ctx, mcp.CallToolRequest{})
	if err != nil {
		t.Fatalf("RequireMailCapability() error = %v", err)
	}
	if called {
		t.Fatal("send handler was called with send disabled")
	}
	if !result.IsError || !strings.Contains(result.Content[0].(mcp.TextContent).Text, "send") {
		t.Fatalf("result = %#v, want exact send capability error", result)
	}

	result, err = RequireMailCapability(MailCapabilityDraft, next)(ctx, mcp.CallToolRequest{})
	if err != nil || result.IsError || !called {
		t.Fatalf("draft result=%#v err=%v called=%v", result, err, called)
	}
}
