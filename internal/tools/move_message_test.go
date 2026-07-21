package tools

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/desek/outlook-local-mcp/internal/auth"
	"github.com/desek/outlook-local-mcp/internal/graph"
	"github.com/desek/outlook-local-mcp/internal/resource"
	"github.com/mark3labs/mcp-go/mcp"
	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"
)

// TestMoveMessageUsesExactRoutesAndReturnsNewReference verifies own and shared
// moves resolve the destination, dispatch once, and replace the stale source
// reference with Graph's destination message identity.
func TestMoveMessageUsesExactRoutesAndReturnsNewReference(t *testing.T) {
	for _, shared := range []bool{false, true} {
		t.Run(map[bool]string{false: "own", true: "shared"}[shared], func(t *testing.T) {
			var paths []string
			client, server := newRealSDKGraphClient(t, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				paths = append(paths, request.Method+" "+request.URL.EscapedPath())
				w.Header().Set("Content-Type", "application/json")
				if request.Method == http.MethodGet {
					_, _ = w.Write([]byte(`{"id":"ordinary+1","displayName":"Projects"}`))
					return
				}
				bodyReader := io.Reader(request.Body)
				if request.Header.Get("Content-Encoding") == "gzip" {
					compressed, gzipErr := gzip.NewReader(request.Body)
					if gzipErr != nil {
						t.Fatal(gzipErr)
					}
					defer compressed.Close()
					bodyReader = compressed
				}
				rawBody, _ := io.ReadAll(bodyReader)
				var body map[string]any
				_ = json.Unmarshal(rawBody, &body)
				if body["DestinationId"] != "ordinary+1" {
					t.Errorf("move body = %s (%+v)", rawBody, body)
				}
				_, _ = w.Write([]byte(`{"id":"moved+1","subject":"Filed"}`))
			}))
			defer server.Close()
			ctx, codec, target := moveMessageContext(t, client, shared)
			destinationRef := signFolderClass(t, codec, target, "ordinary+1", resource.MailFolderClassOrdinary)
			request := mcp.CallToolRequest{}
			request.Params.Arguments = map[string]any{"destination_folder_ref": destinationRef}
			result, err := NewHandleMoveMessage(graph.RetryConfig{}, time.Second, &codec)(ctx, request)
			if err != nil || result.IsError {
				t.Fatalf("result = %+v, error = %v", result, err)
			}
			prefix := "/v1.0/me"
			if shared {
				prefix = "/v1.0/users/shared%2Bops%40example.com"
			}
			itemSuffix := "/mailFolders/ordinary+1|POST " + prefix + "/messages/source+1/move"
			if shared {
				itemSuffix = "/mailFolders/ordinary%2B1|POST " + prefix + "/messages/source%2B1/move"
			}
			want := "GET " + prefix + itemSuffix
			if got := strings.Join(paths, "|"); got != want {
				t.Fatalf("paths = %v, want %s", paths, want)
			}
			text := result.Content[0].(mcp.TextContent).Text
			if strings.Contains(text, "source+1") || !strings.Contains(text, "Resource Ref: v1.") {
				t.Fatalf("move confirmation = %s", text)
			}
		})
	}
}

// TestMoveMessageRejectsReservedAndCrossTargetDestinationsLocally verifies
// semantic and provenance bypasses produce zero Graph traffic.
func TestMoveMessageRejectsReservedAndCrossTargetDestinationsLocally(t *testing.T) {
	var calls int
	client, server := newRealSDKGraphClient(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls++ }))
	defer server.Close()
	ctx, codec, target := moveMessageContext(t, client, true)
	other := target
	other.ResourceID = "44444444-4444-4444-8444-444444444444"
	tests := []string{
		signFolderClass(t, codec, target, "archive-id", resource.MailFolderClassReserved),
		signFolderClass(t, codec, other, "ordinary-id", resource.MailFolderClassOrdinary),
	}
	for _, reference := range tests {
		request := mcp.CallToolRequest{}
		request.Params.Arguments = map[string]any{"destination_folder_ref": reference}
		result, err := NewHandleMoveMessage(graph.RetryConfig{}, time.Second, &codec)(ctx, request)
		if err != nil || !result.IsError {
			t.Fatalf("result = %+v, error = %v", result, err)
		}
	}
	if calls != 0 {
		t.Fatalf("Graph calls = %d, want zero", calls)
	}
}

// TestListFoldersIncludeRefsClassifiesMoveDestinations verifies explicit
// discovery signs authoritative ordinary and reserved folder classes.
func TestListFoldersIncludeRefsClassifiesMoveDestinations(t *testing.T) {
	client, server := newRealSDKGraphClient(t, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch request.URL.EscapedPath() {
		case "/v1.0/me/mailFolders":
			_, _ = w.Write([]byte(`{"value":[{"id":"ordinary-1","displayName":"Projects"},{"id":"archive-id","displayName":"Archive"}]}`))
		case "/v1.0/me/mailFolders/archive":
			_, _ = w.Write([]byte(`{"id":"archive-id"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"code":"ErrorItemNotFound","message":"absent"}}`))
		}
	}))
	defer server.Close()
	ctx, codec, _ := moveMessageContext(t, client, false)
	routed, _ := graph.RoutedTargetFromContext(ctx)
	routed.Claims = nil
	ctx = graph.WithRoutedTarget(ctx, routed)
	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]any{"include_refs": true, "output": "summary"}
	result, err := NewHandleListMailFolders(graph.RetryConfig{}, 5*time.Second, &codec)(ctx, request)
	if err != nil || result.IsError {
		t.Fatalf("result = %+v, error = %v", result, err)
	}
	var items []map[string]any
	if err := json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &items); err != nil {
		t.Fatal(err)
	}
	if items[0]["destination_class"] != "ordinary" || items[1]["destination_class"] != "reserved" {
		t.Fatalf("classified folders = %+v", items)
	}
}

// moveMessageContext builds one own or shared target with verified source
// message claims and a request-local reference codec.
func moveMessageContext(t *testing.T, client *msgraphsdk.GraphServiceClient, shared bool) (context.Context, resource.ReferenceCodec, resource.Target) {
	t.Helper()
	key, err := resource.LoadOrCreateSigningKey(filepath.Join(t.TempDir(), "reference.key"))
	if err != nil {
		t.Fatal(err)
	}
	codec := resource.NewReferenceCodec(key)
	var target resource.Target
	if shared {
		alias, err := resource.NewMailAlias("22222222-2222-4222-8222-222222222222", "finance-mail", "shared+ops@example.com")
		if err != nil {
			t.Fatal(err)
		}
		alias.Policy.Move = true
		target, err = resource.TargetFromMailAlias("11111111-1111-4111-8111-111111111111", alias)
		if err != nil {
			t.Fatal(err)
		}
	} else {
		target, err = resource.NewOwnTarget("11111111-1111-4111-8111-111111111111", resource.TargetFamilyMail, resource.MailActionPolicy{Move: true})
		if err != nil {
			t.Fatal(err)
		}
	}
	root, err := graph.SelectUserRoot(client, target)
	if err != nil {
		t.Fatal(err)
	}
	claims := resource.ReferenceClaims{
		AccountID: target.AccountID, ResourceID: target.ResourceID, ResourceKind: target.Kind,
		MailboxView: target.View, ItemKind: resource.ItemKindMessage,
		GraphIDChain: []resource.GraphID{{Kind: resource.ItemKindMessage, ID: "source+1"}},
	}
	ctx := auth.WithGraphClient(context.Background(), client)
	ctx = graph.WithRoutedTarget(ctx, graph.RoutedTarget{Target: target, Root: root, Claims: &claims, Reauthorize: func() error { return nil }})
	return ctx, codec, target
}

// signFolderClass signs one same-target folder classification for move tests.
func signFolderClass(t *testing.T, codec resource.ReferenceCodec, target resource.Target, id string, class resource.MailFolderClass) string {
	t.Helper()
	reference, err := codec.Sign(resource.ReferenceClaims{
		AccountID: target.AccountID, ResourceID: target.ResourceID, ResourceKind: target.Kind,
		MailboxView: target.View, ItemKind: resource.ItemKindMailFolder, MailFolderClass: class,
		GraphIDChain: []resource.GraphID{{Kind: resource.ItemKindMailFolder, ID: id}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return reference
}
