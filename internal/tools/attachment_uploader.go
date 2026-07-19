package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/desek/outlook-local-mcp/internal/graph"
)

const attachmentChunkSize int64 = 3 * 1024 * 1024

// uploadRangeResponse is the resumable-session progress shape returned by
// Microsoft Graph after an incomplete attachment range upload.
type uploadRangeResponse struct {
	NextExpectedRanges []string `json:"nextExpectedRanges"`
}

// uploadAttachmentRanges uploads one file sequentially to an Outlook opaque
// upload-session URL. It never adds Graph authorization because the URL itself
// carries short-lived bearer authority.
func uploadAttachmentRanges(ctx context.Context, client *http.Client, uploadURL string, file *os.File, size int64, retryCfg graph.RetryConfig) (string, error) {
	for start := int64(0); start < size; {
		length := min(attachmentChunkSize, size-start)
		end := start + length - 1
		resp, reconciledStart, reconciled, err := putAttachmentRange(ctx, client, uploadURL, file, start, end, size, retryCfg)
		if err != nil {
			return "", err
		}
		if reconciled {
			start = reconciledStart
			continue
		}
		if resp.StatusCode == http.StatusCreated {
			_ = resp.Body.Close()
			return resp.Header.Get("Location"), nil
		}
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
			data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
			_ = resp.Body.Close()
			return "", fmt.Errorf("upload attachment range returned %s: %s", resp.Status, strings.TrimSpace(string(data)))
		}
		var state uploadRangeResponse
		err = json.NewDecoder(resp.Body).Decode(&state)
		_ = resp.Body.Close()
		if err != nil {
			return "", fmt.Errorf("parse next expected attachment range: %w", err)
		}
		if len(state.NextExpectedRanges) == 0 {
			return "", fmt.Errorf("attachment upload response omitted nextExpectedRanges")
		}
		next := strings.TrimSuffix(state.NextExpectedRanges[0], "-")
		nextStart, parseErr := strconv.ParseInt(next, 10, 64)
		if parseErr != nil {
			return "", fmt.Errorf("parse next attachment offset: %w", parseErr)
		}
		if nextStart != end+1 || nextStart > size {
			return "", fmt.Errorf("attachment upload returned non-sequential next offset %d after %d", nextStart, end)
		}
		start = nextStart
	}
	return "", fmt.Errorf("attachment upload ended without completion response")
}

// putAttachmentRange uploads one idempotent range and retries transport errors
// plus HTTP 429/503/504 responses. Retrying the same range is safe for an
// Outlook upload session and never restarts or skips the server-reported offset.
func putAttachmentRange(ctx context.Context, client *http.Client, uploadURL string, file *os.File, start, end, size int64, retryCfg graph.RetryConfig) (*http.Response, int64, bool, error) {
	length := end - start + 1
	for attempt := 0; attempt <= retryCfg.MaxRetries; attempt++ {
		body := io.NewSectionReader(file, start, length)
		req, err := http.NewRequestWithContext(ctx, http.MethodPut, uploadURL, body)
		if err != nil {
			return nil, 0, false, fmt.Errorf("create attachment range request: %w", err)
		}
		req.ContentLength = length
		req.Header.Set("Content-Type", "application/octet-stream")
		req.Header.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, size))
		resp, callErr := client.Do(req)
		if callErr == nil && !retryableAttachmentStatus(resp.StatusCode) {
			return resp, 0, false, nil
		}
		if resp != nil {
			_ = resp.Body.Close()
		}
		if attempt == retryCfg.MaxRetries {
			if callErr != nil {
				return nil, 0, false, fmt.Errorf("upload attachment range: %w", callErr)
			}
			return nil, 0, false, fmt.Errorf("upload attachment range exhausted retries after HTTP %d", resp.StatusCode)
		}
		if err := waitAttachmentRetry(ctx, retryCfg.InitialBackoff, attempt); err != nil {
			return nil, 0, false, err
		}
		nextStart, reconcileErr := attachmentUploadOffset(ctx, client, uploadURL)
		if reconcileErr != nil {
			return nil, 0, false, reconcileErr
		}
		switch nextStart {
		case start:
			continue
		case end + 1:
			return nil, nextStart, true, nil
		default:
			return nil, 0, false, fmt.Errorf("attachment upload status returned unexpected next offset %d", nextStart)
		}
	}
	return nil, 0, false, fmt.Errorf("upload attachment range exhausted retries")
}

// attachmentUploadOffset reads the upload session's server-reported recovery
// offset after a retryable range failure. The opaque URL is never included in
// returned errors, logs, or persisted state.
func attachmentUploadOffset(ctx context.Context, client *http.Client, uploadURL string) (int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, uploadURL, nil)
	if err != nil {
		return 0, fmt.Errorf("create attachment upload status request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("read attachment upload status: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("attachment upload status returned HTTP %d", resp.StatusCode)
	}
	var state uploadRangeResponse
	if err := json.NewDecoder(resp.Body).Decode(&state); err != nil || len(state.NextExpectedRanges) == 0 {
		return 0, fmt.Errorf("attachment upload status omitted nextExpectedRanges")
	}
	next := strings.TrimSuffix(state.NextExpectedRanges[0], "-")
	nextStart, err := strconv.ParseInt(next, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse attachment upload recovery offset: %w", err)
	}
	return nextStart, nil
}

// retryableAttachmentStatus reports transient upload-session HTTP responses.
func retryableAttachmentStatus(status int) bool {
	return status == http.StatusTooManyRequests || status == http.StatusServiceUnavailable || status == http.StatusGatewayTimeout
}

// waitAttachmentRetry waits with bounded exponential backoff while honoring
// MCP cancellation and the operation deadline.
func waitAttachmentRetry(ctx context.Context, initial time.Duration, attempt int) error {
	if initial <= 0 {
		initial = 100 * time.Millisecond
	}
	wait := initial * time.Duration(1<<min(attempt, 8))
	if wait > graph.MaxRetryWait {
		wait = graph.MaxRetryWait
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
