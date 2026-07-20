package resource

import (
	"fmt"
	"os"
	"path/filepath"
)

// RotateSigningKey explicitly replaces the persisted signing authority at
// path. Every reference signed by the previous key becomes invalid.
//
// Parameters:
//   - path: absolute filesystem path of the binary signing-key file.
//
// Returns the newly persisted key or an error for invalid paths, randomness,
// or filesystem failures. Its side effect is an atomic key-file replacement.
func RotateSigningKey(path string) (SigningKey, error) {
	if path == "" {
		return SigningKey{}, fmt.Errorf("signing key path must not be empty")
	}
	key, err := newSigningKey()
	if err != nil {
		return SigningKey{}, err
	}
	if err := writeSigningKeyAtomic(path, key); err != nil {
		return SigningKey{}, err
	}
	return key, nil
}

// writeSigningKeyAtomic writes key to a private temporary file beside path and
// renames it over the current key. It cleans up temporary data on failure.
func writeSigningKeyAtomic(path string, key SigningKey) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create signing key directory: %w", err)
	}
	tmp, err := os.CreateTemp(dir, "resource-reference-*.key.tmp")
	if err != nil {
		return fmt.Errorf("create temporary signing key: %w", err)
	}
	tmpPath := tmp.Name()
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()        //nolint:errcheck // best-effort cleanup after chmod failure
		os.Remove(tmpPath) //nolint:errcheck // best-effort cleanup after chmod failure
		return fmt.Errorf("protect temporary signing key: %w", err)
	}
	if _, err := tmp.Write(key.bytes[:]); err != nil {
		tmp.Close()        //nolint:errcheck // the write error is authoritative
		os.Remove(tmpPath) //nolint:errcheck // best-effort cleanup after write failure
		return fmt.Errorf("write temporary signing key: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath) //nolint:errcheck // best-effort cleanup after close failure
		return fmt.Errorf("close temporary signing key: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath) //nolint:errcheck // best-effort cleanup after rename failure
		return fmt.Errorf("replace signing key: %w", err)
	}
	return nil
}
