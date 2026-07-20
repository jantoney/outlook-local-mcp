package resource

import "testing"

// TestTargetValidateRejectsKindViewAndRoutingMismatch verifies invalid target
// states fail before any Graph builder can be selected.
func TestTargetValidateRejectsKindViewAndRoutingMismatch(t *testing.T) {
	accountID := AccountID("11111111-1111-4111-8111-111111111111")
	resourceID := ResourceID("22222222-2222-4222-8222-222222222222")
	tests := []Target{
		{},
		{AccountID: accountID, ResourceID: resourceID, Kind: ResourceKindMailbox, View: MailboxViewRecipient, Owner: "owner@example.com"},
		{AccountID: accountID, ResourceID: resourceID, Kind: ResourceKindMailbox, View: MailboxViewOwner},
		{AccountID: accountID, ResourceID: resourceID, Kind: ResourceKindMountedCalendar, View: MailboxViewRecipient, Owner: "owner@example.com"},
		{AccountID: accountID, ResourceID: OwnMailboxResourceID, Kind: ResourceKindOwnMailbox, View: MailboxViewRecipient, Owner: "unexpected@example.com"},
	}
	for index, target := range tests {
		if err := target.Validate(); err == nil {
			t.Fatalf("target %d Validate() error = nil: %+v", index, target)
		}
	}
}

// TestTargetAllowsExactCapability verifies target-local policies are neither
// widened by family nor interpreted cumulatively.
func TestTargetAllowsExactCapability(t *testing.T) {
	accountID := AccountID("11111111-1111-4111-8111-111111111111")
	mail, err := NewOwnTarget(accountID, TargetFamilyMail, MailActionPolicy{Archive: true})
	if err != nil {
		t.Fatalf("NewOwnTarget() error = %v", err)
	}
	if !mail.Allows(TargetCapability(MailCapabilityArchive)) || mail.Allows(TargetCapability(MailCapabilityRead)) {
		t.Fatalf("mail capability decision widened policy: %+v", mail.MailPolicy)
	}
	calendar, err := NewOwnTarget(accountID, TargetFamilyCalendar, MailActionPolicy{})
	if err != nil {
		t.Fatalf("NewOwnTarget() error = %v", err)
	}
	if !calendar.Allows(TargetCapabilityManage) || calendar.Allows(TargetCapability(MailCapabilitySend)) {
		t.Fatalf("own calendar capability decision is invalid")
	}
}
