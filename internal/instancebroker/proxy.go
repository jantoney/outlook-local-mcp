package instancebroker

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// Proxy bridges one harness-owned stdio connection to the authoritative
// broker. It preserves raw JSON-RPC IDs and serializes all stdout writes.
type Proxy struct {
	identity   Identity
	executable string
	arguments  []string
	clientID   string
	input      io.Reader
	output     io.Writer

	outputMu     sync.Mutex
	stateMu      sync.RWMutex
	client       *privateClient
	session      string
	catalog      map[string]struct{}
	initRaw      []byte
	readyRaw     []byte
	generation   uint64
	recoverMu    sync.Mutex
	listenCancel context.CancelFunc
}

// NewProxy creates a stdio broker proxy. It makes no network calls until Run
// and returns an error only if a secure random client lease ID cannot be made.
func NewProxy(identity Identity, executable string, arguments []string, input io.Reader, output io.Writer) (*Proxy, error) {
	clientID, err := randomToken()
	if err != nil {
		return nil, err
	}
	return &Proxy{identity: identity, executable: executable, arguments: append([]string(nil), arguments...), clientID: clientID, input: input, output: output, catalog: make(map[string]struct{})}, nil
}

// Run reads newline-delimited JSON-RPC messages until stdin closes. It renews
// the broker lease in the background and waits for dispatched requests before
// returning. Malformed input receives a local JSON-RPC parse error.
func (p *Proxy) Run(ctx context.Context) error {
	if err := p.connect(ctx, true); err != nil {
		return err
	}
	leaseCtx, cancelLease := context.WithCancel(ctx)
	defer cancelLease()
	go p.heartbeat(leaseCtx)

	scanner := bufio.NewScanner(p.input)
	scanner.Buffer(make([]byte, 4096), 16*1024*1024)
	var requests sync.WaitGroup
	for scanner.Scan() {
		raw := append([]byte(nil), scanner.Bytes()...)
		var envelope requestEnvelope
		if err := json.Unmarshal(raw, &envelope); err != nil {
			p.emit([]byte(`{"jsonrpc":"2.0","id":null,"error":{"code":-32700,"message":"Parse error"}}`))
			continue
		}
		if envelope.Method == "initialize" {
			if err := p.initialize(ctx, raw, true); err != nil {
				return err
			}
			continue
		}
		if envelope.Method == "notifications/initialized" {
			p.stateMu.Lock()
			p.readyRaw = append([]byte(nil), raw...)
			p.stateMu.Unlock()
		}
		requests.Add(1)
		go func() {
			defer requests.Done()
			p.forward(ctx, raw)
		}()
	}
	requests.Wait()
	p.closeLease()
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read MCP stdio: %w", err)
	}
	return nil
}

// forward sends one raw message and applies conservative transport recovery.
// Read requests may be replayed once; all other requests receive a retry error.
func (p *Proxy) forward(ctx context.Context, raw []byte) {
	client, session, generation, catalog := p.snapshot()
	_, err := client.post(ctx, raw, session, p.emit)
	if err == nil {
		return
	}
	if !replayDecision(raw, catalog) {
		// Restore transport for the agent's explicit retry, but never submit the
		// ambiguous operation itself a second time.
		_ = p.recover(ctx, generation)
		if response := retryRequiredResponse(raw); response != nil {
			p.emit(response)
		}
		return
	}
	if recoverErr := p.recover(ctx, generation); recoverErr != nil {
		p.emit(transportErrorResponse(raw, recoverErr))
		return
	}
	client, session, _, _ = p.snapshot()
	if _, retryErr := client.post(ctx, raw, session, p.emit); retryErr != nil {
		p.emit(transportErrorResponse(raw, retryErr))
	}
}

// initialize starts a broker MCP session and optionally forwards its response.
// It caches the harness request so a replacement broker session can be rebuilt.
func (p *Proxy) initialize(ctx context.Context, raw []byte, emit bool) error {
	p.stateMu.RLock()
	client := p.client
	p.stateMu.RUnlock()
	var response []byte
	session, err := client.post(ctx, raw, "", func(value []byte) { response = append([]byte(nil), value...) })
	if err != nil {
		return fmt.Errorf("initialize broker MCP session: %w", err)
	}
	if session == "" {
		return fmt.Errorf("initialize broker MCP session: missing session ID")
	}
	p.stateMu.Lock()
	p.session = session
	p.initRaw = append([]byte(nil), raw...)
	p.generation++
	p.startListenerLocked(ctx)
	p.stateMu.Unlock()
	if emit && len(response) > 0 {
		p.emit(response)
	}
	return nil
}

// recover serializes broker reconnection and MCP session reconstruction. If a
// concurrent request already advanced generation, no duplicate recovery occurs.
func (p *Proxy) recover(ctx context.Context, failedGeneration uint64) error {
	p.recoverMu.Lock()
	defer p.recoverMu.Unlock()
	p.stateMu.RLock()
	currentGeneration := p.generation
	initRaw := append([]byte(nil), p.initRaw...)
	readyRaw := append([]byte(nil), p.readyRaw...)
	p.stateMu.RUnlock()
	if currentGeneration != failedGeneration {
		return nil
	}
	if err := p.connect(ctx, true); err != nil {
		return err
	}
	if len(initRaw) == 0 {
		return fmt.Errorf("broker failed before MCP initialization")
	}
	if err := p.initialize(ctx, initRaw, false); err != nil {
		return err
	}
	if len(readyRaw) > 0 {
		client, session, _, _ := p.snapshot()
		if _, err := client.post(ctx, readyRaw, session, func([]byte) {}); err != nil {
			return fmt.Errorf("restore initialized notification: %w", err)
		}
	}
	return nil
}

// snapshot returns a concurrency-safe transport snapshot and a copy of the
// replay catalog so callers never hold locks during network I/O.
func (p *Proxy) snapshot() (*privateClient, string, uint64, map[string]struct{}) {
	p.stateMu.RLock()
	defer p.stateMu.RUnlock()
	catalog := make(map[string]struct{}, len(p.catalog))
	for identity := range p.catalog {
		catalog[identity] = struct{}{}
	}
	return p.client, p.session, p.generation, catalog
}

// emit writes one compact JSON-RPC message followed by a newline to stdout.
func (p *Proxy) emit(raw []byte) {
	if len(raw) == 0 {
		return
	}
	p.outputMu.Lock()
	defer p.outputMu.Unlock()
	_, _ = p.output.Write(append(append([]byte(nil), raw...), '\n'))
}

// transportErrorResponse converts a terminal recovery failure to a JSON-RPC
// error preserving the original request ID.
func transportErrorResponse(raw []byte, cause error) []byte {
	var request requestEnvelope
	if json.Unmarshal(raw, &request) != nil || len(request.ID) == 0 || request.Method == "" {
		return nil
	}
	response := map[string]any{"jsonrpc": "2.0", "id": request.ID, "error": map[string]any{"code": -32072, "message": "Outlook MCP broker recovery failed: " + cause.Error()}}
	encoded, _ := json.Marshal(response)
	return encoded
}

// heartbeat renews the proxy lease until ctx ends. Failures are best-effort;
// an actual MCP request performs authoritative recovery.
func (p *Proxy) heartbeat(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.stateMu.RLock()
			client := p.client
			p.stateMu.RUnlock()
			leaseCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			_ = client.lease(leaseCtx, false)
			cancel()
		}
	}
}

// closeLease best-effort removes this proxy and stops its SSE listener.
func (p *Proxy) closeLease() {
	p.stateMu.Lock()
	if p.listenCancel != nil {
		p.listenCancel()
	}
	client := p.client
	p.stateMu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = client.lease(ctx, true)
}

// startListenerLocked replaces the session SSE listener. The caller must hold
// stateMu for writing.
func (p *Proxy) startListenerLocked(parent context.Context) {
	if p.listenCancel != nil {
		p.listenCancel()
	}
	ctx, cancel := context.WithCancel(parent)
	p.listenCancel = cancel
	client, session := p.client, p.session
	go func() { _ = client.listen(ctx, session, p.emit) }()
}

// DefaultIO returns the process stdio streams used by the executable entry
// point. It exists to keep command wiring explicit and testable.
func DefaultIO() (io.Reader, io.Writer) {
	return os.Stdin, os.Stdout
}
