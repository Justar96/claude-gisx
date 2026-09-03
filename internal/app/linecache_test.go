package app

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// claudeDir() resolves through the home dir, so point HOME at a temp dir to
// keep tests off the real ~/.claude.
func isolateHome(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
}

func TestLimitsRoundTripReusesOpenWindow(t *testing.T) {
	isolateHome(t)
	now := time.Now()
	saveLimits(&rateLimits{
		FiveHour: &rateBucket{UsedPercentage: 42, ResetsAt: now.Add(2 * time.Hour).Unix()},
		SevenDay: &rateBucket{UsedPercentage: 71, ResetsAt: now.Add(72 * time.Hour).Unix()},
	})

	got := loadLimits(now)
	if got == nil || got.FiveHour == nil || got.SevenDay == nil {
		t.Fatalf("expected both buckets restored, got %+v", got)
	}
	if got.FiveHour.UsedPercentage != 42 || got.SevenDay.UsedPercentage != 71 {
		t.Errorf("percentages not preserved: %+v %+v", got.FiveHour, got.SevenDay)
	}
}

func TestLimitsDropsRolledOverWindow(t *testing.T) {
	isolateHome(t)
	now := time.Now()
	saveLimits(&rateLimits{
		// 5h window already expired — its percentage is no longer true.
		FiveHour: &rateBucket{UsedPercentage: 90, ResetsAt: now.Add(-time.Minute).Unix()},
		SevenDay: &rateBucket{UsedPercentage: 30, ResetsAt: now.Add(48 * time.Hour).Unix()},
	})

	got := loadLimits(now)
	if got == nil {
		t.Fatal("expected the still-open 7d bucket to survive")
	}
	if got.FiveHour != nil {
		t.Errorf("expired 5h bucket should be dropped, got %+v", got.FiveHour)
	}
	if got.SevenDay == nil || got.SevenDay.UsedPercentage != 30 {
		t.Errorf("7d bucket should be intact, got %+v", got.SevenDay)
	}
}

func TestLimitsNilWhenAllExpiredOrMissing(t *testing.T) {
	isolateHome(t)
	now := time.Now()

	if got := loadLimits(now); got != nil {
		t.Errorf("no cache file should yield nil, got %+v", got)
	}

	saveLimits(&rateLimits{
		FiveHour: &rateBucket{UsedPercentage: 90, ResetsAt: now.Add(-time.Hour).Unix()},
	})
	if got := loadLimits(now); got != nil {
		t.Errorf("fully expired cache should yield nil, got %+v", got)
	}

	// A bucket with no resets_at can't be checked for rollover, so it's never reused.
	saveLimits(&rateLimits{FiveHour: &rateBucket{UsedPercentage: 55}})
	if got := loadLimits(now); got != nil {
		t.Errorf("bucket without resets_at should not be reused, got %+v", got)
	}
}

func TestLimitsSurvivesCorruptCache(t *testing.T) {
	isolateHome(t)
	p := limitsCachePath()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	// What a half-written file from a killed render used to look like.
	if err := os.WriteFile(p, []byte(`{"five_hour":{"used_per`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := loadLimits(time.Now()); got != nil {
		t.Errorf("truncated cache should yield nil, got %+v", got)
	}
}

func TestWriteFileAtomicReplacesAndLeavesNoTemps(t *testing.T) {
	isolateHome(t)
	p := filepath.Join(lineCacheDir(), "probe.json")
	if err := writeFileAtomic(p, []byte("first")); err != nil {
		t.Fatal(err)
	}
	if err := writeFileAtomic(p, []byte("second")); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "second" {
		t.Errorf("want %q, got %q", "second", raw)
	}
	ents, err := os.ReadDir(lineCacheDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(ents) != 1 {
		t.Errorf("expected only the final file, got %d entries", len(ents))
	}
}

func TestUsageBackoffSuppressesRepeatFetches(t *testing.T) {
	isolateHome(t)
	if fetchBackedOff() {
		t.Error("no failure recorded yet, should not back off")
	}
	if err := writeFileAtomic(usageFailPath(), nil); err != nil {
		t.Fatal(err)
	}
	if !fetchBackedOff() {
		t.Error("a just-recorded failure should back off")
	}

	old := time.Now().Add(-usageBackoff - time.Minute)
	if err := os.Chtimes(usageFailPath(), old, old); err != nil {
		t.Fatal(err)
	}
	if fetchBackedOff() {
		t.Error("an expired failure marker should allow a retry")
	}
}

func TestSweepRemovesStaleTempsButSparesInFlight(t *testing.T) {
	isolateHome(t)
	dir := lineCacheDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	base := "statusline-limits.json"
	stale := filepath.Join(dir, base+".999999")
	fresh := filepath.Join(dir, base+".111111")
	unrelated := filepath.Join(dir, "statusline-usage.fail")
	for _, p := range []string{stale, fresh, unrelated} {
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	old := time.Now().Add(-tempTTL - time.Minute)
	if err := os.Chtimes(stale, old, old); err != nil {
		t.Fatal(err)
	}

	if err := writeFileAtomic(filepath.Join(dir, base), []byte("{}")); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Error("abandoned temp file should have been swept")
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Error("a temp file young enough to be in flight must be spared")
	}
	if _, err := os.Stat(unrelated); err != nil {
		t.Error("sweep must not touch files that aren't temps of this target")
	}
}

func TestRemoveLegacyCacheTakesOnlyOurFiles(t *testing.T) {
	root := t.TempDir()
	old := filepath.Join(root, "claude")
	if err := os.MkdirAll(old, 0o755); err != nil {
		t.Fatal(err)
	}
	ours := filepath.Join(old, "statusline-extra-cache.json")
	theirs := filepath.Join(old, "some-other-tool.json")
	for _, p := range []string{ours, theirs} {
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	removeLegacyCache(root)

	if _, err := os.Stat(ours); !os.IsNotExist(err) {
		t.Error("our orphaned cache file should be gone")
	}
	if _, err := os.Stat(theirs); err != nil {
		t.Error("another tool's file in /tmp/claude must be left alone")
	}
	if _, err := os.Stat(old); err != nil {
		t.Error("directory must survive while it still holds someone else's files")
	}

	// With only our files gone, a second pass on an emptied dir reclaims it.
	if err := os.Remove(theirs); err != nil {
		t.Fatal(err)
	}
	removeLegacyCache(root)
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Error("emptied legacy dir should be removed")
	}
}

func TestCacheDirIsPerUserAndPrivate(t *testing.T) {
	root := t.TempDir()
	t.Setenv("TMPDIR", root)
	d := cacheDir()
	if d == filepath.Join(root, "claude") {
		t.Error("cache dir must not be the shared /tmp/claude path")
	}
	if uid := os.Getuid(); uid >= 0 && !strings.Contains(d, strconv.Itoa(uid)) {
		t.Errorf("cache dir should be scoped to the uid, got %q", d)
	}
}

func TestLimitsRoundTripsSpendLimit(t *testing.T) {
	isolateHome(t)
	now := time.Now()
	saveLimits(&rateLimits{SpendLimit: &rateBucket{UsedPercentage: 120, ResetsAt: now.Add(time.Hour).Unix()}})
	got := loadLimits(now)
	if got == nil || got.SpendLimit == nil || int(got.SpendLimit.UsedPercentage) != 120 {
		t.Fatalf("spend_limit did not survive the round trip: %+v", got)
	}
}
