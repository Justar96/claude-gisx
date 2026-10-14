package app

import (
	"os"
	"testing"
	"time"
)

// os.Executable is the test binary here; spawning it would rerun the suite.
func init() { startRefresh = func(string) {} }

func recordRefreshes(t *testing.T) *[]string {
	t.Helper()
	var got []string
	old := startRefresh
	startRefresh = func(kind string) { got = append(got, kind) }
	t.Cleanup(func() { startRefresh = old })
	return &got
}

func TestClaimLockIsExclusiveUntilStale(t *testing.T) {
	isolateHome(t)
	p := refreshLockPath("usage")
	if !claimLock(p) {
		t.Fatal("first claim should win")
	}
	if claimLock(p) {
		t.Error("a held lock must not be claimed twice")
	}
	old := time.Now().Add(-refreshLockTTL - time.Second)
	if err := os.Chtimes(p, old, old); err != nil {
		t.Fatal(err)
	}
	if !claimLock(p) {
		t.Error("a stale lock should be taken over")
	}
}

func TestFetchUsageServesStaleCacheAndRefreshes(t *testing.T) {
	isolateHome(t)
	got := recordRefreshes(t)
	raw := []byte(`{"extra_usage":{"is_enabled":true,"used_credits":150,"monthly_limit":1000}}`)
	if err := writeFileAtomic(usageCachePath(), raw); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-usageFresh - time.Minute)
	if err := os.Chtimes(usageCachePath(), old, old); err != nil {
		t.Fatal(err)
	}

	u := fetchUsage()
	if u == nil || u.Extra == nil || u.Extra.Used != "1.50" {
		t.Fatalf("stale cache should still render, got %+v", u)
	}
	if len(*got) != 1 || (*got)[0] != "usage" {
		t.Errorf("want one usage refresh, got %v", *got)
	}

	*got = nil
	if err := writeFileAtomic(usageFailPath(), nil); err != nil {
		t.Fatal(err)
	}
	fetchUsage()
	if len(*got) != 0 {
		t.Errorf("a recent failure should back off, got %v", *got)
	}
}

func TestFetchUsageFreshCacheSkipsRefresh(t *testing.T) {
	isolateHome(t)
	got := recordRefreshes(t)
	if err := writeFileAtomic(usageCachePath(), []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	fetchUsage()
	if len(*got) != 0 {
		t.Errorf("fresh cache must not refresh, got %v", *got)
	}
}
