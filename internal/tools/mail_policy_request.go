package tools

import (
	"fmt"

	"github.com/desek/outlook-local-mcp/internal/auth"
)

// mailPolicyFromArguments applies boolean action fields from args to current.
// It returns the complete replacement policy and whether at least one action
// field was supplied. Unknown arguments are handled by aggregate dispatch.
func mailPolicyFromArguments(current auth.MailActionPolicy, args map[string]any) (auth.MailActionPolicy, bool, error) {
	policy := current
	changed := false
	fields := []struct {
		name  string
		value *bool
	}{
		{name: "read", value: &policy.Read},
		{name: "draft", value: &policy.Draft},
		{name: "move", value: &policy.Move},
		{name: "archive", value: &policy.Archive},
		{name: "trash", value: &policy.Trash},
		{name: "restore", value: &policy.Restore},
		{name: "permanent_delete", value: &policy.PermanentDelete},
		{name: "send", value: &policy.Send},
	}
	for _, field := range fields {
		raw, ok := args[field.name]
		if !ok {
			continue
		}
		value, ok := raw.(bool)
		if !ok {
			return current, false, fmt.Errorf("parameter %s must be a boolean", field.name)
		}
		*field.value = value
		changed = true
	}
	return policy, changed, nil
}
