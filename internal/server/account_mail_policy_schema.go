package server

import (
	"github.com/mark3labs/mcp-go/mcp"
)

// mailPolicySchema returns the optional boolean fields used to patch one
// target's independent action policy. The caller supplies the target selector.
func mailPolicySchema() []mcp.ToolOption {
	return []mcp.ToolOption{
		mcp.WithBoolean("read", mcp.Description("Allow mail, folder, conversation, and attachment reads.")),
		mcp.WithBoolean("draft", mcp.Description("Allow draft lifecycle and draft attachment writes without sending.")),
		mcp.WithBoolean("move", mcp.Description("Allow moves to ordinary folders only.")),
		mcp.WithBoolean("archive", mcp.Description("Allow semantic Outlook Archive moves.")),
		mcp.WithBoolean("trash", mcp.Description("Allow reversible moves to Deleted Items.")),
		mcp.WithBoolean("restore", mcp.Description("Allow restoration from Deleted Items to an ordinary folder.")),
		mcp.WithBoolean("permanent_delete", mcp.Description("Allow separately confirmed permanent deletion; disabled by default.")),
		mcp.WithBoolean("send", mcp.Description("Allow delivery of an existing draft after human review.")),
	}
}
