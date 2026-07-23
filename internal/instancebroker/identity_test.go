package instancebroker

import (
	"path/filepath"
	"testing"

	"github.com/desek/outlook-local-mcp/internal/config"
)

// TestIdentitySeparatesExecutableAndStateRealms verifies the two isolation
// dimensions required for independent harness installations and databases.
func TestIdentitySeparatesExecutableAndStateRealms(t *testing.T) {
	base := config.Config{AccountsPath: filepath.Join(t.TempDir(), "accounts.json"), AuthRecordPath: filepath.Join(t.TempDir(), "auth.json"), CacheName: "cache", TokenStorage: "file"}
	first, err := NewIdentity(filepath.Join(t.TempDir(), "outlook-mcp.exe"), base)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewIdentity(filepath.Join(t.TempDir(), "outlook-mcp.exe"), base)
	if err != nil {
		t.Fatal(err)
	}
	if first.Key == second.Key {
		t.Fatal("different executable paths unexpectedly share an instance identity")
	}
	if first.StateKey != second.StateKey {
		t.Fatal("same store unexpectedly produced different state realms")
	}

	differentStore := base
	differentStore.AccountsPath = filepath.Join(t.TempDir(), "accounts.json")
	third, err := NewIdentity(first.ExecutablePath, differentStore)
	if err != nil {
		t.Fatal(err)
	}
	if first.StateKey == third.StateKey || first.Key == third.Key {
		t.Fatal("different account stores unexpectedly share broker identity")
	}
}

// TestIdentityChangesWithBehavior verifies incompatible tool behavior does not
// attach to an existing broker endpoint.
func TestIdentityChangesWithBehavior(t *testing.T) {
	cfg := config.Config{AccountsPath: filepath.Join(t.TempDir(), "accounts.json"), AuthRecordPath: filepath.Join(t.TempDir(), "auth.json")}
	first, err := NewIdentity("outlook-mcp.exe", cfg)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ReadOnly = true
	second, err := NewIdentity("outlook-mcp.exe", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if first.Key == second.Key || first.StateKey != second.StateKey {
		t.Fatal("behavior change should change instance identity but retain state realm")
	}
}
