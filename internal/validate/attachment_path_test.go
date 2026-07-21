package validate

import (
	"os"
	"path/filepath"
	"testing"
)

// TestAttachmentPathWithinRoot verifies allowlisted regular files are accepted.
func TestAttachmentPathWithinRoot(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "report.pdf")
	if err := os.WriteFile(path, []byte("report"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := AttachmentPath(path, []string{root})
	if err != nil {
		t.Fatalf("AttachmentPath() error = %v", err)
	}
	if got != path {
		t.Fatalf("AttachmentPath() = %q, want %q", got, path)
	}
}

// TestAttachmentPathOutsideRootRejected verifies paths outside every root fail.
func TestAttachmentPathOutsideRootRejected(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := AttachmentPath(outside, []string{root}); err == nil {
		t.Fatal("AttachmentPath() returned nil error for outside path")
	}
}

// TestAttachmentPathDisabledWithoutRoots verifies local-file access fails closed.
func TestAttachmentPathDisabledWithoutRoots(t *testing.T) {
	t.Parallel()

	if _, err := AttachmentPath("report.pdf", nil); err == nil {
		t.Fatal("AttachmentPath() returned nil error without roots")
	}
}
