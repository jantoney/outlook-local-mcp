package resource

import (
	"os"
	"path/filepath"
	"testing"
)

// TestSigningKeyPersistsAcrossRestart verifies that loading the same key path
// returns the original key rather than generating new signing authority.
func TestSigningKeyPersistsAcrossRestart(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "resource-reference.key")
	first, err := LoadOrCreateSigningKey(path)
	if err != nil {
		t.Fatalf("LoadOrCreateSigningKey() error = %v", err)
	}
	second, err := LoadOrCreateSigningKey(path)
	if err != nil {
		t.Fatalf("second LoadOrCreateSigningKey() error = %v", err)
	}
	if first != second {
		t.Fatal("signing key changed across restart")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if info.Size() != SigningKeySize {
		t.Fatalf("key file size = %d, want %d", info.Size(), SigningKeySize)
	}
}

// TestRotateSigningKeyReplacesPersistedAuthority verifies that explicit key
// rotation changes the authority and that subsequent loads return the new key.
func TestRotateSigningKeyReplacesPersistedAuthority(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "resource-reference.key")
	before, err := LoadOrCreateSigningKey(path)
	if err != nil {
		t.Fatalf("LoadOrCreateSigningKey() error = %v", err)
	}
	after, err := RotateSigningKey(path)
	if err != nil {
		t.Fatalf("RotateSigningKey() error = %v", err)
	}
	if before == after {
		t.Fatal("RotateSigningKey() reused the existing key")
	}
	reloaded, err := LoadOrCreateSigningKey(path)
	if err != nil {
		t.Fatalf("reload error = %v", err)
	}
	if after != reloaded {
		t.Fatal("rotated key was not persisted")
	}
}

// TestLoadSigningKeyRejectsCorruption verifies that malformed persisted key
// material fails closed instead of silently rotating outstanding references.
func TestLoadSigningKeyRejectsCorruption(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "resource-reference.key")
	if err := os.WriteFile(path, []byte("too-short"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := LoadOrCreateSigningKey(path); err == nil {
		t.Fatal("LoadOrCreateSigningKey() error = nil, want corrupted-key error")
	}
}
