package instancebroker

import (
	"context"
	"fmt"
	"os"
	"time"
)

// connect resolves a healthy compatible endpoint, optionally spawning a broker
// candidate, and refreshes the proxy's authoritative replay catalog.
func (p *Proxy) connect(ctx context.Context, allowSpawn bool) error {
	endpoint, metadata, err := p.findHealthy(ctx)
	if err != nil && allowSpawn {
		if spawnErr := SpawnBroker(p.executable, p.arguments); spawnErr != nil {
			return spawnErr
		}
		endpoint, metadata, err = p.waitForHealthy(ctx, 10*time.Second)
	}
	if err != nil {
		return err
	}
	client := newPrivateClient(endpoint, p.clientID)
	leaseCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	err = client.lease(leaseCtx, false)
	cancel()
	if err != nil {
		return fmt.Errorf("register broker lease: %w", err)
	}
	catalog := make(map[string]struct{}, len(metadata.ReplayableTools))
	for _, identity := range metadata.ReplayableTools {
		catalog[identity] = struct{}{}
	}
	p.stateMu.Lock()
	p.client = client
	p.catalog = catalog
	p.stateMu.Unlock()
	return nil
}

// findHealthy reads the private descriptor and verifies it against authenticated
// live metadata before returning it to a proxy.
func (p *Proxy) findHealthy(ctx context.Context) (Endpoint, Metadata, error) {
	endpoint, err := ReadEndpoint(p.identity.DescriptorPath(), p.identity)
	if err != nil {
		return Endpoint{}, Metadata{}, err
	}
	client := newPrivateClient(endpoint, p.clientID)
	probeCtx, cancel := context.WithTimeout(ctx, 750*time.Millisecond)
	defer cancel()
	metadata, err := client.metadata(probeCtx)
	if err != nil {
		return Endpoint{}, Metadata{}, err
	}
	if metadata.IdentityKey != p.identity.Key || metadata.StateKey != p.identity.StateKey || metadata.Protocol != ProtocolVersion {
		return Endpoint{}, Metadata{}, fmt.Errorf("broker metadata identity mismatch")
	}
	return endpoint, metadata, nil
}

// waitForHealthy polls while simultaneous candidates complete election and
// startup. It exits early when ctx is canceled or timeout elapses.
func (p *Proxy) waitForHealthy(ctx context.Context, timeout time.Duration) (Endpoint, Metadata, error) {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		endpoint, metadata, err := p.findHealthy(ctx)
		if err == nil {
			return endpoint, metadata, nil
		}
		lastErr = err
		select {
		case <-ctx.Done():
			return Endpoint{}, Metadata{}, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	if lastErr == nil {
		lastErr = os.ErrDeadlineExceeded
	}
	return Endpoint{}, Metadata{}, fmt.Errorf("broker did not become healthy: %w", lastErr)
}
