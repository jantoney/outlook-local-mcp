package server

import (
	"testing"

	"github.com/desek/outlook-local-mcp/internal/auth"
	"github.com/desek/outlook-local-mcp/internal/config"
	"github.com/desek/outlook-local-mcp/internal/tools"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

// TestBuildMailVerbsIsStaticAndDeclaresCapabilities verifies discovery does
// not depend on global defaults and every non-help verb advertises an exact guard.
func TestBuildMailVerbsIsStaticAndDeclaresCapabilities(t *testing.T) {
	identity := func(handler mcpserver.ToolHandlerFunc) mcpserver.ToolHandlerFunc { return handler }
	verbs, _ := buildMailVerbs(mailVerbsConfig{cfg: config.Config{}, authMW: identity, accountResolverMW: identity})
	want := map[string]auth.MailCapability{
		"list_folders": auth.MailCapabilityRead, "list_messages": auth.MailCapabilityRead,
		"get_message": auth.MailCapabilityRead, "search_messages": auth.MailCapabilityRead,
		"get_conversation": auth.MailCapabilityRead, "list_attachments": auth.MailCapabilityRead,
		"get_attachment": auth.MailCapabilityRead, "create_draft": auth.MailCapabilityDraft,
		"create_reply_draft": auth.MailCapabilityDraft, "create_forward_draft": auth.MailCapabilityDraft,
		"update_draft": auth.MailCapabilityDraft, "delete_draft": auth.MailCapabilityDraft,
		"add_attachment": auth.MailCapabilityDraft, "send_draft": auth.MailCapabilitySend,
	}
	seen := make(map[string]bool)
	for _, verb := range verbs {
		if verb.Name == "help" {
			continue
		}
		seen[verb.Name] = true
		if verb.RequiredCapability != string(want[verb.Name]) {
			t.Errorf("%s required capability = %q, want %q", verb.Name, verb.RequiredCapability, want[verb.Name])
		}
	}
	if len(seen) != len(want) {
		t.Fatalf("registered verbs = %v, want %d profile-gated verbs", seen, len(want))
	}
}

// TestCR0066VerbAnnotations verifies the three new verbs explicitly declare
// their per-verb safety and open-world semantics.
func TestCR0066VerbAnnotations(t *testing.T) {
	identity := func(handler mcpserver.ToolHandlerFunc) mcpserver.ToolHandlerFunc { return handler }
	mailVerbs, _ := buildMailVerbs(mailVerbsConfig{cfg: config.Config{}, authMW: identity, accountResolverMW: identity})
	accountVerbs, _ := buildAccountVerbs(accountVerbsConfig{cfg: config.Config{}, authMW: identity})
	assertVerb := func(verbs []tools.Verb, name string, idempotent, openWorld bool) {
		t.Helper()
		for _, verb := range verbs {
			if verb.Name != name {
				continue
			}
			tool := mcp.NewTool(name, verb.Annotations...)
			ann := tool.Annotations
			if ann.ReadOnlyHint == nil || *ann.ReadOnlyHint || ann.DestructiveHint == nil || *ann.DestructiveHint ||
				ann.IdempotentHint == nil || *ann.IdempotentHint != idempotent || ann.OpenWorldHint == nil || *ann.OpenWorldHint != openWorld {
				t.Fatalf("%s annotations = %+v", name, ann)
			}
			return
		}
		t.Fatalf("verb %s not registered", name)
	}
	assertVerb(mailVerbs, "add_attachment", false, true)
	assertVerb(mailVerbs, "send_draft", false, true)
	assertVerb(accountVerbs, "set_mail_profile", true, false)
}
