package auth

import (
	"sync"
	"testing"

	"github.com/desek/outlook-local-mcp/internal/resource"
)

// TestRegistryReadsReturnDeepSnapshots verifies callers cannot mutate live
// account state or its slice-backed policies after a registry lock is released.
func TestRegistryReadsReturnDeepSnapshots(t *testing.T) {
	r := NewAccountRegistry()
	original := &AccountEntry{
		Label:           "work",
		Authenticated:   true,
		Scopes:          []string{"Mail.Read"},
		CalendarAliases: []resource.CalendarAlias{{Alias: "calendar"}},
		MailAliases:     []resource.MailAlias{{Alias: "mail"}},
	}
	if err := r.Add(original); err != nil {
		t.Fatal(err)
	}

	original.Authenticated = false
	original.Scopes[0] = "mutated-before-read"
	got, ok := r.Get("work")
	if !ok {
		t.Fatal("Get returned ok=false")
	}
	got.Authenticated = false
	got.Scopes[0] = "mutated-after-read"
	got.CalendarAliases[0].Alias = "changed"
	got.MailAliases[0].Alias = "changed"

	again, _ := r.Get("work")
	if !again.Authenticated || again.Scopes[0] != "Mail.Read" ||
		again.CalendarAliases[0].Alias != "calendar" || again.MailAliases[0].Alias != "mail" {
		t.Fatalf("registry state escaped through snapshot: %+v", again)
	}
}

// TestRegistryConcurrentSnapshotsAndUpdates exercises the lock-owned snapshot
// boundary used when policy changes race with active request resolution.
func TestRegistryConcurrentSnapshotsAndUpdates(t *testing.T) {
	r := NewAccountRegistry()
	if err := r.Add(&AccountEntry{Label: "work", Authenticated: true, Scopes: []string{"Mail.Read"}}); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for index := 0; index < 20; index++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			_, _ = r.Get("work")
			_ = r.List()
			_ = r.ListAuthenticated()
		}()
		go func(enabled bool) {
			defer wg.Done()
			if err := r.Update("work", func(entry *AccountEntry) {
				entry.Authenticated = enabled
				entry.Scopes = []string{"Mail.ReadWrite"}
			}); err != nil {
				t.Errorf("Update: %v", err)
			}
		}(index%2 == 0)
	}
	wg.Wait()
}
