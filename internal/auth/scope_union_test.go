package auth

import (
	"slices"
	"testing"
)

// TestOAuthScopeUnion verifies deterministic, order-independent scope union
// calculation across multiple configured policies.
func TestOAuthScopeUnion(t *testing.T) {
	t.Parallel()

	left := OAuthScopeUnion(
		[]string{"Mail.Send", "User.Read", "Mail.Read.Shared"},
		[]string{"Calendars.ReadWrite", "Mail.Send", "Mail.Read"},
	)
	right := OAuthScopeUnion(
		[]string{"Mail.Read", "Calendars.ReadWrite"},
		[]string{"Mail.Read.Shared", "User.Read", "Mail.Send"},
	)
	want := []string{"User.Read", "Calendars.ReadWrite", "Mail.Read", "Mail.Read.Shared", "Mail.Send"}
	if !slices.Equal(left, want) {
		t.Fatalf("OAuthScopeUnion() = %v, want %v", left, want)
	}
	if !slices.Equal(left, right) {
		t.Fatalf("OAuthScopeUnion() depends on policy order: %v != %v", left, right)
	}
}

// TestOAuthScopeUnionUnknownScopes verifies that future Graph scopes remain
// deterministic rather than being discarded by the canonical union.
func TestOAuthScopeUnionUnknownScopes(t *testing.T) {
	t.Parallel()

	got := OAuthScopeUnion([]string{"Z.Future", "A.Future", "Z.Future"})
	want := []string{"A.Future", "Z.Future"}
	if !slices.Equal(got, want) {
		t.Fatalf("OAuthScopeUnion() = %v, want %v", got, want)
	}
}

// TestOAuthScopeSetEqual verifies that reauthentication decisions compare
// semantic unions rather than incidental slice ordering or duplication.
func TestOAuthScopeSetEqual(t *testing.T) {
	t.Parallel()

	if !OAuthScopeSetEqual([]string{"Mail.Read", "User.Read"}, []string{"User.Read", "Mail.Read", "Mail.Read"}) {
		t.Fatal("equivalent OAuth scope unions were reported as changed")
	}
	if OAuthScopeSetEqual([]string{"User.Read"}, []string{"User.Read", "Mail.Read"}) {
		t.Fatal("different OAuth scope unions were reported as unchanged")
	}
}
