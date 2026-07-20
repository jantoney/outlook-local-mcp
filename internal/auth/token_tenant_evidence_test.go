package auth

import (
	"path/filepath"
	"testing"

	"github.com/AzureAD/microsoft-authentication-library-for-go/apps/public"
)

// TestTokenTenantContextFromAuthState verifies both supported authentication
// implementations classify only their persisted validated tenant evidence.
func TestTokenTenantContextFromAuthState(t *testing.T) {
	t.Parallel()

	t.Run("Azure Identity authentication record", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), "record.json")
		record := testRecord()
		record.TenantID = MicrosoftConsumerTenantID
		if err := SaveAuthRecord(path, record); err != nil {
			t.Fatalf("SaveAuthRecord() error = %v", err)
		}
		if got := TokenTenantContextFromAuthState("browser", path); got != TokenTenantPersonal {
			t.Fatalf("TokenTenantContextFromAuthState() = %q, want %q", got, TokenTenantPersonal)
		}
	})

	t.Run("MSAL account realm", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), "account.json")
		if err := saveAuthCodeAccount(path, public.Account{Realm: "00000000-0000-4000-8000-000000000001"}); err != nil {
			t.Fatalf("saveAuthCodeAccount() error = %v", err)
		}
		if got := TokenTenantContextFromAuthState("auth_code", path); got != TokenTenantOrganizational {
			t.Fatalf("TokenTenantContextFromAuthState() = %q, want %q", got, TokenTenantOrganizational)
		}
	})

	t.Run("missing evidence", func(t *testing.T) {
		t.Parallel()
		if got := TokenTenantContextFromAuthState("device_code", filepath.Join(t.TempDir(), "missing.json")); got != TokenTenantUnknown {
			t.Fatalf("TokenTenantContextFromAuthState() = %q, want %q", got, TokenTenantUnknown)
		}
	})
}
