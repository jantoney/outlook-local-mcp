package resource

import "testing"

// TestCalendarAliasIdentityLifecycle verifies rename and reselection preserve
// identity while fresh creation produces an unrelated revocation boundary.
func TestCalendarAliasIdentityLifecycle(t *testing.T) {
	t.Parallel()

	id, err := NewResourceID()
	if err != nil {
		t.Fatalf("NewID() error = %v", err)
	}
	original, err := NewMountedCalendar(id, "team", "owner@example.com", "mounted-1", CalendarProfileRead)
	if err != nil {
		t.Fatalf("NewMountedCalendar() error = %v", err)
	}
	renamed, err := original.Rename("finance")
	if err != nil {
		t.Fatalf("Rename() error = %v", err)
	}
	reselected, err := renamed.ReselectMounted("mounted-2")
	if err != nil {
		t.Fatalf("ReselectMounted() error = %v", err)
	}
	if renamed.ResourceID != original.ResourceID || reselected.ResourceID != original.ResourceID {
		t.Fatal("rename or reselection changed immutable resource identity")
	}
	if reselected.MountedCalendarID != "mounted-2" {
		t.Fatalf("mounted ID = %q, want mounted-2", reselected.MountedCalendarID)
	}
}

// TestValidateCalendarCompatibility verifies unsupported tenant/view/profile
// combinations fail before any route can be built.
func TestValidateCalendarCompatibility(t *testing.T) {
	t.Parallel()

	id, err := NewResourceID()
	if err != nil {
		t.Fatalf("NewResourceID() error = %v", err)
	}
	owner, err := NewOwnerPrimaryCalendar(id, "owner", "owner@example.com", CalendarProfileRead)
	if err != nil {
		t.Fatalf("NewOwnerPrimaryCalendar() error = %v", err)
	}
	if err := ValidateCalendarCompatibility(owner, "personal"); err == nil {
		t.Fatal("personal owner-primary compatibility succeeded")
	}
	mounted, err := NewMountedCalendar(id, "mounted", "owner@example.com", "id", CalendarProfileManage)
	if err != nil {
		t.Fatalf("NewMountedCalendar() error = %v", err)
	}
	if err := ValidateCalendarCompatibility(mounted, "unknown"); err == nil {
		t.Fatal("unknown mounted manage compatibility succeeded")
	}
	if err := ValidateCalendarCompatibility(mounted, "organizational"); err != nil {
		t.Fatalf("organizational mounted manage compatibility error = %v", err)
	}
}

// TestCalendarAliasRejectsInvalidCombinations verifies kind/view/profile
// compatibility fails before persistence or Graph routing.
func TestCalendarAliasRejectsInvalidCombinations(t *testing.T) {
	t.Parallel()

	id, err := NewResourceID()
	if err != nil {
		t.Fatalf("NewID() error = %v", err)
	}
	tests := []CalendarAlias{
		{ResourceID: id, Alias: "owner", Owner: "owner@example.com", Kind: CalendarKindOwnerPrimary, View: MailboxViewOwner, Profile: CalendarProfileManage},
		{ResourceID: id, Alias: "mounted", Owner: "owner@example.com", Kind: CalendarKindMounted, View: MailboxViewRecipient, Profile: CalendarProfileRead},
		{ResourceID: id, Alias: "wrong-view", Owner: "owner@example.com", Kind: CalendarKindMounted, View: MailboxViewOwner, MountedCalendarID: "id", Profile: CalendarProfileRead},
	}
	for _, candidate := range tests {
		if err := candidate.Validate(); err == nil {
			t.Fatalf("Validate(%+v) succeeded, want incompatibility error", candidate)
		}
	}
}
