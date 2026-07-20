package resource

import "testing"

// TestMailAliasLifecycle verifies default-off policy and immutable identity
// across the supported rename and policy replacement operations.
func TestMailAliasLifecycle(t *testing.T) {
	id, err := NewResourceID()
	if err != nil {
		t.Fatalf("NewResourceID() error = %v", err)
	}
	alias, err := NewMailAlias(id, "finance", "finance@example.com")
	if err != nil {
		t.Fatalf("NewMailAlias() error = %v", err)
	}
	if alias.Policy != (MailActionPolicy{}) {
		t.Fatalf("new alias policy = %+v, want every action disabled", alias.Policy)
	}
	renamed, err := alias.Rename("finance-team")
	if err != nil {
		t.Fatalf("Rename() error = %v", err)
	}
	if renamed.ResourceID != id || renamed.Owner != alias.Owner || renamed.Kind != ResourceKindMailbox || renamed.View != MailboxViewOwner {
		t.Fatalf("rename changed immutable identity: before=%+v after=%+v", alias, renamed)
	}
	updated := renamed.WithPolicy(MailActionPolicy{Read: true, Archive: true})
	if updated.ResourceID != id || !updated.Policy.Read || !updated.Policy.Archive || updated.Policy.Send {
		t.Fatalf("WithPolicy() = %+v", updated)
	}
}

// TestMailAliasCompatibilityFailsClosed verifies personal and unknown token
// contexts are rejected before a shared-mail route can be constructed.
func TestMailAliasCompatibilityFailsClosed(t *testing.T) {
	id, err := NewResourceID()
	if err != nil {
		t.Fatalf("NewResourceID() error = %v", err)
	}
	alias, err := NewMailAlias(id, "finance", "finance@example.com")
	if err != nil {
		t.Fatalf("NewMailAlias() error = %v", err)
	}
	for _, context := range []string{"personal", "unknown", ""} {
		if err := ValidateMailCompatibility(alias, context); err == nil {
			t.Fatalf("ValidateMailCompatibility(%q) error = nil", context)
		}
	}
	if err := ValidateMailCompatibility(alias, "organizational"); err != nil {
		t.Fatalf("organizational compatibility error = %v", err)
	}
}
