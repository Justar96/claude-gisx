package app

import (
	"encoding/json"
	"testing"
	"time"
)

// An offline machine records a check with no tag. That attempt — not the
// missing tag — must decide whether to ask GitHub again.
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

	got := recordRefreshes(t)
	if tag := availableUpdate(6 * time.Hour); tag != "" {
		t.Errorf("expected no tag from the recorded attempt, got %q", tag)
	}
	if len(*got) != 0 {
		t.Errorf("a recent attempt should not refresh, got %v", *got)
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

	got := recordRefreshes(t)
	availableUpdate(6 * time.Hour)
	if len(*got) != 1 || (*got)[0] != "update" {
		t.Errorf("an expired attempt should start a refresh, got %v", *got)
	}
}

// A stale cached tag still shows while the refresh runs.
func TestAvailableUpdateShowsStaleTag(t *testing.T) {
	isolateHome(t)
	t.Setenv("TMPDIR", t.TempDir())
	t.Setenv("CLAUDE_GISX_NO_UPDATE_CHECK", "")
	old := Version
	Version = "v1.0.0"
	t.Cleanup(func() { Version = old })
	recordRefreshes(t)

	raw, _ := json.Marshal(updateCache{Tag: "v1.2.0", CheckedAt: time.Now().Add(-24 * time.Hour).Unix()})
	if err := writeFileAtomic(updateCachePath(), raw); err != nil {
		t.Fatal(err)
	}
	if tag := availableUpdate(6 * time.Hour); tag != "v1.2.0" {
		t.Errorf("want v1.2.0, got %q", tag)
	}
}
