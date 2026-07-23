package instancebroker

import (
	"path/filepath"
	"testing"

	"github.com/desek/outlook-local-mcp/internal/config"
)

// TestClaimElectsOneBrokerAndGuardsStateRealm verifies a second compatible
// candidate and an incompatible executable sharing the store cannot initialize.
func TestClaimElectsOneBrokerAndGuardsStateRealm(t *testing.T) {
	directory := t.TempDir()
	cfg := config.Config{AccountsPath: filepath.Join(directory, "accounts.json"), AuthRecordPath: filepath.Join(directory, "auth.json"), CacheName: "claim-test-" + filepath.Base(directory)}
	first, err := NewIdentity(filepath.Join(directory, "first.exe"), cfg)
	if err != nil {
		t.Fatal(err)
	}
	listener, endpoint, err := Claim(first)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	defer RemoveEndpoint(first.DescriptorPath(), endpoint)

	if secondListener, _, secondErr := Claim(first); secondErr == nil {
		_ = secondListener.Close()
		t.Fatal("second compatible broker unexpectedly won election")
	}
	incompatible, err := NewIdentity(filepath.Join(directory, "second.exe"), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if incompatibleListener, _, incompatibleErr := Claim(incompatible); incompatibleErr == nil {
		_ = incompatibleListener.Close()
		t.Fatal("incompatible broker unexpectedly acquired shared state realm")
	}
}
