package server

import (
	"testing"

	"github.com/desek/outlook-local-mcp/internal/auth"
	"github.com/desek/outlook-local-mcp/internal/config"
	"github.com/desek/outlook-local-mcp/internal/tools"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

// TestBuildMailVerbsIsStaticAndDeclaresProfiles verifies discovery does not
// depend on global default flags and every non-help verb advertises its guard.
func TestBuildMailVerbsIsStaticAndDeclaresProfiles(t *testing.T) {
	identity := func(handler mcpserver.ToolHandlerFunc) mcpserver.ToolHandlerFunc { return handler }
	verbs, _ := buildMailVerbs(mailVerbsConfig{cfg: config.Config{}, authMW: identity, accountResolverMW: identity})
	want := map[string]auth.MailProfile{
		"list_folders": auth.MailProfileRead, "list_messages": auth.MailProfileRead,
		"get_message": auth.MailProfileRead, "search_messages": auth.MailProfileRead,
		"get_conversation": auth.MailProfileRead, "list_attachments": auth.MailProfileRead,
		"get_attachment": auth.MailProfileRead, "create_draft": auth.MailProfileManage,
		"create_reply_draft": auth.MailProfileManage, "create_forward_draft": auth.MailProfileManage,
		"update_draft": auth.MailProfileManage, "delete_draft": auth.MailProfileManage,
		"add_attachment": auth.MailProfileManage, "send_draft": auth.MailProfileSend,
	}
	seen := make(map[string]bool)
	for _, verb := range verbs {
		if verb.Name == "help" {
			continue
		}
		seen[verb.Name] = true
		if verb.MinimumProfile != want[verb.Name].String() {
			t.Errorf("%s minimum profile = %q, want %q", verb.Name, verb.MinimumProfile, want[verb.Name])
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
