package instancebroker

import (
	"sync"
	"time"
)

// Leases tracks recently active stdio proxies so the broker can remain alive
// while any harness is connected and shut down after a bounded idle grace.
type Leases struct {
	mu       sync.Mutex
	clients  map[string]time.Time
	ttl      time.Duration
	lastSeen time.Time
}

// NewLeases creates an empty lease set using ttl to expire crashed proxies.
// A non-positive ttl is replaced with thirty seconds.
func NewLeases(ttl time.Duration) *Leases {
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	now := time.Now()
	return &Leases{clients: make(map[string]time.Time), ttl: ttl, lastSeen: now}
}

// Touch creates or renews clientID and records broker activity. Empty client
// identifiers are ignored. The method is safe for concurrent HTTP requests.
func (l *Leases) Touch(clientID string) {
	if clientID == "" {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	l.clients[clientID] = now
	l.lastSeen = now
}

// Remove ends clientID's lease immediately. Removing an unknown client is a
// no-op and does not reset the idle clock.
func (l *Leases) Remove(clientID string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.clients, clientID)
}

// Count returns the number of unexpired proxy leases at now and prunes stale
// entries as a side effect.
func (l *Leases) Count(now time.Time) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.prune(now)
	return len(l.clients)
}

// Idle reports whether no proxy has remained active for ttl at now. It prunes
// expired leases and is safe for use by a lifecycle monitor.
func (l *Leases) Idle(now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.prune(now)
	return len(l.clients) == 0 && now.Sub(l.lastSeen) >= l.ttl
}

// prune removes clients whose last heartbeat is older than ttl. The caller
// must hold l.mu.
func (l *Leases) prune(now time.Time) {
	for clientID, seen := range l.clients {
		if now.Sub(seen) >= l.ttl {
			delete(l.clients, clientID)
		}
	}
}
