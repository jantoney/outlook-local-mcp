package instancebroker

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
)

// privateClient performs authenticated proxy-to-broker HTTP requests.
type privateClient struct {
	http     *http.Client
	endpoint Endpoint
	clientID string
}

// newPrivateClient constructs a loopback-only broker client. It makes no
// network call and uses the supplied endpoint capability token.
func newPrivateClient(endpoint Endpoint, clientID string) *privateClient {
	return &privateClient{http: &http.Client{}, endpoint: endpoint, clientID: clientID}
}

// metadata fetches broker identity and replay information. Any non-successful
// response is returned as an error and no metadata is trusted.
func (c *privateClient) metadata(ctx context.Context) (Metadata, error) {
	request, err := c.request(ctx, http.MethodGet, "/broker/metadata", nil, "")
	if err != nil {
		return Metadata{}, err
	}
	response, err := c.http.Do(request)
	if err != nil {
		return Metadata{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return Metadata{}, fmt.Errorf("broker metadata status %s", response.Status)
	}
	var metadata Metadata
	if err := decodeJSON(response.Body, &metadata); err != nil {
		return Metadata{}, err
	}
	return metadata, nil
}

// lease renews or removes this proxy's broker lease. It returns transport and
// non-success HTTP failures to the caller.
func (c *privateClient) lease(ctx context.Context, remove bool) error {
	method := http.MethodPost
	if remove {
		method = http.MethodDelete
	}
	request, err := c.request(ctx, method, "/broker/lease", nil, "")
	if err != nil {
		return err
	}
	response, err := c.http.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("broker lease status %s", response.Status)
	}
	return nil
}

// post sends one JSON-RPC message and invokes emit for each JSON response or
// SSE data event. It returns session IDs created by initialize responses.
func (c *privateClient) post(ctx context.Context, raw []byte, sessionID string, emit func([]byte)) (string, error) {
	request, err := c.request(ctx, http.MethodPost, "/mcp", bytes.NewReader(raw), sessionID)
	if err != nil {
		return "", err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	response, err := c.http.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 512))
		return "", fmt.Errorf("broker MCP status %s: %s", response.Status, strings.TrimSpace(string(body)))
	}
	if response.StatusCode == http.StatusAccepted || response.StatusCode == http.StatusNoContent {
		return response.Header.Get("Mcp-Session-Id"), nil
	}
	mediaType, _, _ := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if mediaType == "text/event-stream" {
		if err := readSSE(response.Body, emit); err != nil {
			return "", err
		}
	} else {
		body, err := io.ReadAll(response.Body)
		if err != nil {
			return "", err
		}
		if len(bytes.TrimSpace(body)) > 0 {
			emit(bytes.TrimSpace(body))
		}
	}
	return response.Header.Get("Mcp-Session-Id"), nil
}

// listen consumes the server's long-lived SSE channel for unsolicited MCP
// notifications and server-to-client requests until cancellation or failure.
func (c *privateClient) listen(ctx context.Context, sessionID string, emit func([]byte)) error {
	request, err := c.request(ctx, http.MethodGet, "/mcp", nil, sessionID)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "text/event-stream")
	response, err := c.http.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("broker listen status %s", response.Status)
	}
	return readSSE(response.Body, emit)
}

// request constructs one authenticated private request. The bearer secret is
// only placed in an HTTP header and is never added to URLs or errors.
func (c *privateClient) request(ctx context.Context, method, path string, body io.Reader, sessionID string) (*http.Request, error) {
	request, err := http.NewRequestWithContext(ctx, method, "http://"+c.endpoint.Address+path, body)
	if err != nil {
		return nil, err
	}
	request.Header.Set(authorizationHeader, "Bearer "+c.endpoint.Token)
	request.Header.Set(clientHeader, c.clientID)
	if sessionID != "" {
		request.Header.Set("Mcp-Session-Id", sessionID)
	}
	return request, nil
}

// readSSE emits each data field as an independent JSON-RPC message. Comments,
// event identifiers, and retry hints are intentionally ignored.
func readSSE(reader io.Reader, emit func([]byte)) error {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 4096), 16*1024*1024)
	var data []string
	flush := func() {
		if len(data) > 0 {
			emit([]byte(strings.Join(data, "\n")))
			data = data[:0]
		}
	}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			flush()
			continue
		}
		if strings.HasPrefix(line, "data:") {
			data = append(data, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	flush()
	return scanner.Err()
}

// decodeJSON decodes exactly one JSON value from reader.
func decodeJSON(reader io.Reader, target any) error {
	return json.NewDecoder(reader).Decode(target)
}
