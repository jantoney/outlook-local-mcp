package instancebroker

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"
)

const (
	authorizationHeader = "Authorization"
	clientHeader        = "X-Outlook-MCP-Proxy"
)

// Metadata is the non-secret broker state used by proxies for diagnostics and
// fail-closed replay classification.
type Metadata struct {
	// IdentityKey identifies the compatible broker instance.
	IdentityKey string `json:"identity_key"`
	// StateKey identifies the persistent account realm.
	StateKey string `json:"state_key"`
	// PID is the authoritative broker process identifier.
	PID int `json:"pid"`
	// Protocol is the private broker protocol version.
	Protocol int `json:"protocol"`
	// Clients is the current count of unexpired stdio proxy leases.
	Clients int `json:"clients"`
	// ReplayableTools lists exact read-only domain.operation identities.
	ReplayableTools []string `json:"replayable_tools"`
}

// NewHandler creates the authenticated private HTTP surface. mcpHandler owns
// MCP protocol handling; all routes, including health, require the capability
// token. Requests renew the calling proxy's lease.
func NewHandler(endpoint Endpoint, identity Identity, leases *Leases, replayableTools []string, mcpHandler http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/mcp", mcpHandler)
	mux.HandleFunc("/broker/metadata", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, Metadata{IdentityKey: identity.Key, StateKey: identity.StateKey, PID: endpoint.PID, Protocol: ProtocolVersion, Clients: leases.Count(time.Now()), ReplayableTools: replayableTools})
	})
	mux.HandleFunc("/broker/lease", func(w http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodDelete {
			leases.Remove(request.Header.Get(clientHeader))
		} else if request.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if !secureEqual(request.Header.Get(authorizationHeader), "Bearer "+endpoint.Token) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		leases.Touch(request.Header.Get(clientHeader))
		w.Header().Set("X-Outlook-MCP-Broker-PID", strconv.Itoa(endpoint.PID))
		mux.ServeHTTP(w, request)
	})
}

// writeJSON serializes value as an HTTP JSON response. Metadata values are
// constructed in memory and serialization errors therefore produce HTTP 500.
func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		http.Error(w, "encode response", http.StatusInternalServerError)
	}
}
