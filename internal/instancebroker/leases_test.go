package instancebroker

import (
	"testing"
	"time"
)

// TestLeasesKeepBrokerAliveUntilExpiry verifies one proxy can leave while
// another continues to hold the shared broker process alive.
func TestLeasesKeepBrokerAliveUntilExpiry(t *testing.T) {
	leases := NewLeases(50 * time.Millisecond)
	leases.Touch("first")
	leases.Touch("second")
	leases.Remove("first")
	if leases.Count(time.Now()) != 1 || leases.Idle(time.Now()) {
		t.Fatal("remaining proxy did not keep broker active")
	}
	if !leases.Idle(time.Now().Add(100 * time.Millisecond)) {
		t.Fatal("expired proxy did not allow broker to become idle")
	}
}
