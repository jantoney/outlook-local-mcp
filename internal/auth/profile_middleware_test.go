package auth

import (
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestRequireMailProfileBlocksInsufficientAccount(t *testing.T) {
	t.Parallel()

	called := false
	next := func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		called = true
		return mcp.NewToolResultText("called"), nil
	}
	ctx := WithAccountInfo(context.Background(), AccountInfo{
		Label: "personal", MailProfile: MailProfileManage,
	})
	result, err := RequireMailProfile(MailProfileSend, next)(ctx, mcp.CallToolRequest{})
	if err != nil {
		t.Fatalf("RequireMailProfile() error = %v", err)
	}
	if called {
		t.Fatal("protected handler was called")
	}
	if !result.IsError || !strings.Contains(result.Content[0].(mcp.TextContent).Text, "mail_send") {
		t.Fatalf("result = %#v, want required profile error", result)
	}
}

func TestRequireMailProfileAllowsSufficientAccount(t *testing.T) {
	t.Parallel()

	called := false
	next := func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		called = true
		return mcp.NewToolResultText("called"), nil
	}
	ctx := WithAccountInfo(context.Background(), AccountInfo{
		Label: "operations", MailProfile: MailProfileSend,
	})
	result, err := RequireMailProfile(MailProfileManage, next)(ctx, mcp.CallToolRequest{})
	if err != nil || result.IsError || !called {
		t.Fatalf("allowed handler result=%#v err=%v called=%v", result, err, called)
	}
}
