package tools

import (
	"strings"

	"github.com/desek/outlook-local-mcp/internal/auth"
)

// formatMailPolicyText renders the enabled actions in stable risk order. It
// returns "none" for the secure zero-value policy.
func formatMailPolicyText(policy auth.MailActionPolicy) string {
	actions := make([]string, 0, 8)
	for _, candidate := range []struct {
		name    string
		enabled bool
	}{
		{name: "read", enabled: policy.Read},
		{name: "draft", enabled: policy.Draft},
		{name: "move", enabled: policy.Move},
		{name: "archive", enabled: policy.Archive},
		{name: "trash", enabled: policy.Trash},
		{name: "restore", enabled: policy.Restore},
		{name: "permanent_delete", enabled: policy.PermanentDelete},
		{name: "send", enabled: policy.Send},
	} {
		if candidate.enabled {
			actions = append(actions, candidate.name)
		}
	}
	if len(actions) == 0 {
		return "none"
	}
	return strings.Join(actions, ",")
}
