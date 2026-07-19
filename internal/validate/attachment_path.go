package validate

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// AttachmentPath resolves a requested local attachment to a canonical regular
// file inside one configured root. It returns an error when upload is disabled,
// the file is unavailable, or canonical containment fails.
func AttachmentPath(path string, roots []string) (string, error) {
	if len(roots) == 0 {
		return "", fmt.Errorf("local attachment upload is disabled: configure OUTLOOK_MCP_ATTACHMENT_ROOTS")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve attachment path: %w", err)
	}
	canonical, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("resolve attachment path: %w", err)
	}
	info, err := os.Stat(canonical)
	if err != nil {
		return "", fmt.Errorf("stat attachment: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("attachment path is not a regular file")
	}
	for _, root := range roots {
		rootAbs, rootErr := filepath.Abs(root)
		if rootErr != nil {
			continue
		}
		rootCanonical, rootErr := filepath.EvalSymlinks(rootAbs)
		if rootErr != nil {
			continue
		}
		rel, relErr := filepath.Rel(rootCanonical, canonical)
		if relErr == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel) {
			return canonical, nil
		}
	}
	return "", fmt.Errorf("attachment path is outside configured roots")
}
