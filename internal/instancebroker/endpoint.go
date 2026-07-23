package instancebroker

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
)

// Endpoint is the private descriptor written by the elected broker and read by
// compatible stdio proxies. Token is a capability secret and must never be
// logged or included in user-visible status output.
type Endpoint struct {
	// IdentityKey binds the endpoint to one behavior-compatible runtime.
	IdentityKey string `json:"identity_key"`
	// StateKey binds the endpoint to one persistent state realm.
	StateKey string `json:"state_key"`
	// Address is the broker's IPv4 loopback host and port.
	Address string `json:"address"`
	// Token authenticates private broker requests.
	Token string `json:"token"`
	// PID identifies the authoritative broker process for diagnostics only.
	PID int `json:"pid"`
	// Protocol is the private broker protocol version.
	Protocol int `json:"protocol"`
}

// Claim attempts the deterministic election bind. A non-nil listener means
// this process won and may initialize mutable state. If another process has
// already bound the address, Claim returns an error without touching state.
func Claim(identity Identity) (net.Listener, Endpoint, error) {
	stateGuard, err := net.Listen("tcp4", identity.StateAddress())
	if err != nil {
		return nil, Endpoint{}, fmt.Errorf("state realm is already owned by another broker: %w", err)
	}
	listener, err := net.Listen("tcp4", identity.ElectionAddress())
	if err != nil {
		_ = stateGuard.Close()
		return nil, Endpoint{}, fmt.Errorf("broker election address %s unavailable: %w", identity.ElectionAddress(), err)
	}
	token, err := randomToken()
	if err != nil {
		_ = listener.Close()
		_ = stateGuard.Close()
		return nil, Endpoint{}, err
	}
	endpoint := Endpoint{IdentityKey: identity.Key, StateKey: identity.StateKey, Address: listener.Addr().String(), Token: token, PID: os.Getpid(), Protocol: ProtocolVersion}
	if err := WriteEndpoint(identity.DescriptorPath(), endpoint); err != nil {
		_ = listener.Close()
		_ = stateGuard.Close()
		return nil, Endpoint{}, err
	}
	return &guardedListener{Listener: listener, stateGuard: stateGuard}, endpoint, nil
}

// guardedListener holds the state-realm guard for exactly as long as the API
// listener is alive and closes both resources together.
type guardedListener struct {
	net.Listener
	stateGuard net.Listener
}

// Close releases both the API election socket and state-realm guard. It
// returns the API close error unless only the guard close failed.
func (l *guardedListener) Close() error {
	apiErr := l.Listener.Close()
	guardErr := l.stateGuard.Close()
	if apiErr != nil {
		return apiErr
	}
	return guardErr
}

// ReadEndpoint loads and validates identity binding from path. It returns an
// error for missing, malformed, incompatible, or empty-secret descriptors.
func ReadEndpoint(path string, identity Identity) (Endpoint, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Endpoint{}, err
	}
	var endpoint Endpoint
	if err := json.Unmarshal(data, &endpoint); err != nil {
		return Endpoint{}, fmt.Errorf("decode broker descriptor: %w", err)
	}
	if endpoint.IdentityKey != identity.Key || endpoint.StateKey != identity.StateKey || endpoint.Protocol != ProtocolVersion || endpoint.Token == "" {
		return Endpoint{}, fmt.Errorf("broker descriptor identity mismatch")
	}
	return endpoint, nil
}

// WriteEndpoint atomically persists endpoint with owner-only permissions. It
// creates the descriptor directory and replaces any stale descriptor.
func WriteEndpoint(path string, endpoint Endpoint) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create broker descriptor directory: %w", err)
	}
	data, err := json.Marshal(endpoint)
	if err != nil {
		return fmt.Errorf("encode broker descriptor: %w", err)
	}
	temporary := path + fmt.Sprintf(".%d.tmp", os.Getpid())
	if err := os.WriteFile(temporary, data, 0o600); err != nil {
		return fmt.Errorf("write broker descriptor: %w", err)
	}
	if err := os.Rename(temporary, path); err != nil {
		_ = os.Remove(temporary)
		return fmt.Errorf("publish broker descriptor: %w", err)
	}
	return nil
}

// RemoveEndpoint removes path only when it still describes this broker. This
// prevents an exiting stale broker from deleting a successor's descriptor.
func RemoveEndpoint(path string, endpoint Endpoint) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var current Endpoint
	if json.Unmarshal(data, &current) == nil && current.PID == endpoint.PID && secureEqual(current.Token, endpoint.Token) {
		_ = os.Remove(path)
	}
}

// randomToken creates a 256-bit URL-safe capability token.
func randomToken() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("generate broker capability token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

// secureEqual compares capability tokens without content-dependent timing.
func secureEqual(left, right string) bool {
	return len(left) == len(right) && subtle.ConstantTimeCompare([]byte(left), []byte(right)) == 1
}
