package tools

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/desek/outlook-local-mcp/internal/resource"
	"github.com/mark3labs/mcp-go/mcp"
)

// TestArchiveAndTrashUseOnlyWellKnownDestinations verifies the separately
// gated semantic verbs construct one move using their fixed Graph destination.
func TestArchiveAndTrashUseOnlyWellKnownDestinations(t *testing.T) {
	tests := []struct {
		name, destination string
		handler           func(time.Duration, *resource.ReferenceCodec) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error)
	}{
		{name: "archive", destination: "archive", handler: NewHandleArchiveMessage},
		{name: "trash", destination: "deleteditems", handler: NewHandleTrashMessage},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var calls int
			client, server := newRealSDKGraphClient(t, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				calls++
				if request.Method != http.MethodPost || request.URL.EscapedPath() != "/v1.0/users/shared%2Bops%40example.com/messages/source%2B1/move" {
					t.Fatalf("request = %s %s", request.Method, request.URL.EscapedPath())
				}
				body := io.Reader(request.Body)
				if request.Header.Get("Content-Encoding") == "gzip" {
					reader, err := gzip.NewReader(request.Body)
					if err != nil {
						t.Fatal(err)
					}
					defer reader.Close()
					body = reader
				}
				var payload map[string]any
				if err := json.NewDecoder(body).Decode(&payload); err != nil || payload["DestinationId"] != test.destination {
					t.Fatalf("move payload = %+v, error = %v", payload, err)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"id":"new+1"}`))
			}))
			defer server.Close()
			ctx, codec, _ := moveMessageContext(t, client, true)
			result, err := test.handler(time.Second, &codec)(ctx, mcp.CallToolRequest{})
			if err != nil || result.IsError || calls != 1 {
				t.Fatalf("result = %+v, calls = %d, error = %v", result, calls, err)
			}
		})
	}
}
