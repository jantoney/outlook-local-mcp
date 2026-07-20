package auth

import "testing"

// TestClassifyTokenTenantContext verifies that only validated tenant GUIDs
// influence account context and that authority aliases fail closed.
func TestClassifyTokenTenantContext(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		tenantID string
		want     TokenTenantContext
	}{
		{name: "consumer tenant", tenantID: "9188040d-6c67-4c5b-b112-36a304b66dad", want: TokenTenantPersonal},
		{name: "consumer tenant uppercase", tenantID: "9188040D-6C67-4C5B-B112-36A304B66DAD", want: TokenTenantPersonal},
		{name: "organizational tenant", tenantID: "00000000-0000-4000-8000-000000000001", want: TokenTenantOrganizational},
		{name: "common authority", tenantID: "common", want: TokenTenantUnknown},
		{name: "organizations authority", tenantID: "organizations", want: TokenTenantUnknown},
		{name: "email is not evidence", tenantID: "person@outlook.com", want: TokenTenantUnknown},
		{name: "malformed guid", tenantID: "00000000-0000-0000-0000-00000000000z", want: TokenTenantUnknown},
		{name: "missing", tenantID: "", want: TokenTenantUnknown},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := ClassifyTokenTenantContext(test.tenantID); got != test.want {
				t.Fatalf("ClassifyTokenTenantContext(%q) = %q, want %q", test.tenantID, got, test.want)
			}
		})
	}
}

// FuzzClassifyTokenTenantContext verifies that arbitrary non-GUID identity
// metadata can never be upgraded to a known tenant context.
func FuzzClassifyTokenTenantContext(f *testing.F) {
	for _, seed := range []string{"", "common", "alice@example.com", "live.com", "tenant-name"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, tenantID string) {
		context := ClassifyTokenTenantContext(tenantID)
		if context != TokenTenantUnknown && !validTenantGUID(tenantID) {
			t.Fatalf("non-GUID tenant %q classified as %q", tenantID, context)
		}
	})
}
