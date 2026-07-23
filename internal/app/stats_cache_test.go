package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Two weeks of samples, enough for renderTokenTrend to have something to say.
func writeStatsFixture(t *testing.T) {
	t.Helper()
	var s statsCache
	for i := 0; i < 14; i++ {
		day := time.Now().AddDate(0, 0, -i).Format("2006-01-02")
		s.DailyModelTokens = append(s.DailyModelTokens, struct {
			Date          string           `json:"date"`
			TokensByModel map[string]int64 `json:"tokensByModel"`
		}{Date: day, TokensByModel: map[string]int64{"opus": int64(1_000_000 + i*1000)}})
	}
	raw, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(claudeDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statsPath(), raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestTokenTrendServesFromCache(t *testing.T) {
	isolateHome(t)
	writeStatsFixture(t)

	first := tokenTrend()
	if first == "" {
		t.Fatal("fixture should produce a trend")
	}
	if _, err := os.Stat(trendCachePath()); err != nil {
		t.Fatal("a cache file should have been written")
	}

	// Swap the cached text, keeping the key: a second call must return the
	// sentinel, proving it never re-parsed the stats file.
	raw, err := os.ReadFile(trendCachePath())
	if err != nil {
		t.Fatal(err)
	}
	var c trendCache
	if err := json.Unmarshal(raw, &c); err != nil {
		t.Fatal(err)
	}
	c.Rendered = "SENTINEL"
	raw, err = json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(trendCachePath(), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	if got := tokenTrend(); got != "SENTINEL" {
		t.Errorf("expected the cached value, got %q", got)
	}
}

func TestTokenTrendKeyCoversSourceAndDate(t *testing.T) {
	isolateHome(t)
	writeStatsFixture(t)
	tokenTrend()

	raw, err := os.ReadFile(trendCachePath())
	if err != nil {
		t.Fatal(err)
	}
	var c trendCache
	if err := json.Unmarshal(raw, &c); err != nil {
		t.Fatal(err)
	}
	// The result ages out on its own, so a key tied only to the file would
	// keep serving a trend that should have expired.
	if !strings.Contains(c.Stamp, time.Now().Format("2006-01-02")) {
		t.Errorf("stamp should include the date, got %q", c.Stamp)
	}
	st, err := os.Stat(statsPath())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(c.Stamp, fmt.Sprint(st.Size())) {
		t.Errorf("stamp should include the source size, got %q", c.Stamp)
	}

	// A rewritten stats file invalidates the entry rather than being ignored.
	c.Rendered = "STALE"
	c.Stamp = "different-key"
	raw, _ = json.Marshal(c)
	if err := os.WriteFile(trendCachePath(), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	if got := tokenTrend(); got == "STALE" {
		t.Error("a mismatched key must force a recompute")
	}
}

func TestTokenTrendNoStatsFile(t *testing.T) {
	isolateHome(t)
	if got := tokenTrend(); got != "" {
		t.Errorf("missing stats file should yield empty, got %q", got)
	}
	if _, err := os.Stat(filepath.Join(lineCacheDir(), "statusline-trend.json")); err == nil {
		t.Error("nothing to cache when there's no source file")
	}
}
