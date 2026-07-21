package tools

import "testing"

// TestValidateSharedMessageContinuationPinsPublicOwnerCollection verifies a
// continuation can change only its query while its public Graph origin and
// decoded owner-view collection path remain immutable.
func TestValidateSharedMessageContinuationPinsPublicOwnerCollection(t *testing.T) {
	owner := "shared+ops@example.com"
	collection := "/v1.0/users/shared+ops@example.com/messages"
	tests := []struct {
		name string
		link string
		ok   bool
	}{
		{name: "valid encoded owner", link: "https://graph.microsoft.com/v1.0/users/shared%2Bops%40example.com/messages?$skiptoken=next", ok: true},
		{name: "hostile host", link: "https://evil.example/v1.0/users/shared%2Bops%40example.com/messages?$skiptoken=next"},
		{name: "Graph suffix on hostile host", link: "https://graph.microsoft.com.evil.example/v1.0/users/shared%2Bops%40example.com/messages?$skiptoken=next"},
		{name: "non TLS", link: "http://graph.microsoft.com/v1.0/users/shared%2Bops%40example.com/messages?$skiptoken=next"},
		{name: "cross owner", link: "https://graph.microsoft.com/v1.0/users/other%40example.com/messages?$skiptoken=next"},
		{name: "recipient view", link: "https://graph.microsoft.com/v1.0/me/messages?$skiptoken=next"},
		{name: "changed collection", link: "https://graph.microsoft.com/v1.0/users/shared%2Bops%40example.com/mailFolders/inbox/messages?$skiptoken=next"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateSharedMessageContinuation(test.link, owner, collection)
			if test.ok && err != nil {
				t.Fatalf("validateSharedMessageContinuation() error = %v", err)
			}
			if !test.ok && err == nil {
				t.Fatal("validateSharedMessageContinuation() succeeded")
			}
		})
	}
	if err := validateSharedMessageContinuation(
		"https://graph.microsoft.com/v1.0/users/shared%2Bops%40example.com/mailFolders/folder%2B1/messages?$skiptoken=next",
		owner,
		"/v1.0/users/shared+ops@example.com/mailFolders/folder+1/messages",
	); err != nil {
		t.Fatalf("folder continuation error = %v", err)
	}
}
