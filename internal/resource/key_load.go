package resource

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// LoadOrCreateSigningKey loads the restart-stable signing key at path or
// creates one when the file does not exist. Existing malformed material fails
// closed and is never rotated implicitly.
//
// Parameters:
//   - path: absolute filesystem path of the binary signing-key file.
//
// Returns the persisted key or an error for invalid paths, filesystem failures,
// corrupted key material, or unavailable secure randomness. Creation makes the
// parent directory private where supported and writes a mode-0600 key file.
func LoadOrCreateSigningKey(path string) (SigningKey, error) {
	key, err := loadSigningKey(path)
	if err == nil {
		return key, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return SigningKey{}, err
	}

	key, err = newSigningKey()
	if err != nil {
		return SigningKey{}, err
	}
	if err := writeNewSigningKey(path, key); err != nil {
		if errors.Is(err, os.ErrExist) {
			return loadSigningKey(path)
		}
		return SigningKey{}, err
	}
	return key, nil
}

// loadSigningKey reads and validates exactly one binary signing key from path.
// It returns os.ErrNotExist through error wrapping when no key exists and has no
// side effects.
func loadSigningKey(path string) (SigningKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return SigningKey{}, fmt.Errorf("read signing key: %w", err)
	}
	if int64(len(data)) != SigningKeySize {
		return SigningKey{}, fmt.Errorf("invalid signing key length %d: want %d", len(data), SigningKeySize)
	}
	var key SigningKey
	copy(key.bytes[:], data)
	return key, nil
}

// writeNewSigningKey persists a newly generated key without overwriting an
// existing authority. It creates the parent directory and returns os.ErrExist
// through wrapping if another process wins concurrent creation.
func writeNewSigningKey(path string, key SigningKey) error {
	if path == "" {
		return fmt.Errorf("signing key path must not be empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create signing key directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create signing key: %w", err)
	}
	if _, err := file.Write(key.bytes[:]); err != nil {
		file.Close() //nolint:errcheck // the write error is authoritative
		return fmt.Errorf("write signing key: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close signing key: %w", err)
	}
	return nil
}
