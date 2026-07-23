package app

import (
	"encoding/json"
	"os"
	"testing"
	"time"
)

// availableUpdate used to retry whenever it had no tag, so an offline machine
// re-dialed GitHub on every render and paid the full timeout each time. A
// recorded attempt — even one that produced no tag — must suppress the retry.
func TestAvailableUpdateHonorsRecordedEmptyAttempt(t *testing.T) {
	isolateHome(t)
	t.Setenv("TMPDIR", t.TempDir())
	t.Setenv("CLAUDE_GISX_NO_UPDATE_CHECK", "")
	old := Version
	Version = "v1.0.0"
	t.Cleanup(func() { Version = old })

	recorded := time.Now().Add(-time.Minute).Unix()
	raw, err := json.Marshal(updateCache{Tag: "", CheckedAt: recorded})
	if err != nil {
		t.Fatal(err)
	}
	if err := writeFileAtomic(updateCachePath(), raw); err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	if tag := availableUpdate(6 * time.Hour); tag != "" {
		t.Errorf("expected no tag from the recorded attempt, got %q", tag)
	}
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Errorf("took %v — that's a network round trip, not a cache hit", elapsed)
	}

	after, err := os.ReadFile(updateCachePath())
	if err != nil {
		t.Fatal(err)
	}
	var c updateCache
	if json.Unmarshal(after, &c) != nil {
		t.Fatal(err)
	}
	if c.CheckedAt != recorded {
		t.Error("the attempt was overwritten, so it re-fetched instead of backing off")
	}
}

// Once the window lapses it must try again rather than back off forever.
func TestAvailableUpdateRetriesAfterWindow(t *testing.T) {
	isolateHome(t)
	t.Setenv("TMPDIR", t.TempDir())
	t.Setenv("CLAUDE_GISX_NO_UPDATE_CHECK", "")
	old := Version
	Version = "v1.0.0"
	t.Cleanup(func() { Version = old })

	stale := time.Now().Add(-24 * time.Hour).Unix()
	raw, err := json.Marshal(updateCache{Tag: "", CheckedAt: stale})
	if err != nil {
		t.Fatal(err)
	}
	if err := writeFileAtomic(updateCachePath(), raw); err != nil {
		t.Fatal(err)
	}

	availableUpdate(6 * time.Hour)

	after, err := os.ReadFile(updateCachePath())
	if err != nil {
		t.Fatal(err)
	}
	var c updateCache
	if json.Unmarshal(after, &c) != nil {
		t.Fatal(err)
	}
	if c.CheckedAt == stale {
		t.Error("an expired attempt should have triggered a fresh check")
	}
}
