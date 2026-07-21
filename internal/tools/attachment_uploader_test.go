package tools

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/desek/outlook-local-mcp/internal/graph"
)

// TestUploadAttachmentRangesUsesSequentialRangesWithoutAuthorization verifies
// upload URLs receive contiguous chunks without a Graph authorization header.
func TestUploadAttachmentRangesUsesSequentialRangesWithoutAuthorization(t *testing.T) {
	t.Parallel()

	const size = int64(3*1024*1024 + 10)
	path := filepath.Join(t.TempDir(), "large.bin")
	if err := os.WriteFile(path, make([]byte, size), 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	var mu sync.Mutex
	var ranges []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if got := r.Header.Get("Authorization"); got != "" {
			t.Errorf("Authorization header = %q, want empty", got)
		}
		ranges = append(ranges, r.Header.Get("Content-Range"))
		if len(ranges) == 1 {
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"nextExpectedRanges":["%d-"]}`, 3*1024*1024)
			return
		}
		w.Header().Set("Location", "attachment-1")
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	location, err := uploadAttachmentRanges(context.Background(), server.Client(), server.URL, file, size, graph.RetryConfig{})
	if err != nil {
		t.Fatalf("uploadAttachmentRanges() error = %v", err)
	}
	if location != "attachment-1" {
		t.Fatalf("location = %q, want attachment-1", location)
	}
	want := []string{
		fmt.Sprintf("bytes 0-%d/%d", 3*1024*1024-1, size),
		fmt.Sprintf("bytes %d-%d/%d", 3*1024*1024, size-1, size),
	}
	if len(ranges) != len(want) || ranges[0] != want[0] || ranges[1] != want[1] {
		t.Fatalf("ranges = %v, want %v", ranges, want)
	}
}

// TestUploadAttachmentRangesRetriesSameTransientRange verifies retryable
// upload-session failures resend the identical range without restarting.
func TestUploadAttachmentRangesRetriesSameTransientRange(t *testing.T) {
	path := filepath.Join(t.TempDir(), "retry.bin")
	if err := os.WriteFile(path, []byte("retry me"), 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	var ranges []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"nextExpectedRanges":["0-"]}`)
			return
		}
		ranges = append(ranges, r.Header.Get("Content-Range"))
		if len(ranges) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Location", "attachment-2")
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()
	location, err := uploadAttachmentRanges(context.Background(), server.Client(), server.URL, file, 8, graph.RetryConfig{MaxRetries: 1, InitialBackoff: time.Millisecond})
	if err != nil || location != "attachment-2" {
		t.Fatalf("location=%q err=%v", location, err)
	}
	if len(ranges) != 2 || ranges[0] != ranges[1] {
		t.Fatalf("retried ranges = %v", ranges)
	}
}

// TestAttachmentIDFromLocation verifies the final Graph Location header is
// reduced to the URL-decoded attachment identifier used for verification.
func TestAttachmentIDFromLocation(t *testing.T) {
	id, err := attachmentIDFromLocation("https://graph.microsoft.com/v1.0/me/messages/draft/attachments/AAMk%2B123")
	if err != nil || id != "AAMk+123" {
		t.Fatalf("attachmentIDFromLocation() = (%q, %v)", id, err)
	}
}
