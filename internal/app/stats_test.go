package app

import (
	"strings"
	"testing"
	"time"
)

func TestRenderTokenTrend(t *testing.T) {
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	// i counts back from the newest day, so i==0 is the day being reported.
	build := func(days int, perDay func(i int) int64) statsCache {
		var s statsCache
		for i := 0; i < days; i++ {
			day := now.AddDate(0, 0, -i).Format("2006-01-02")
			s.DailyModelTokens = append(s.DailyModelTokens, struct {
				Date          string           `json:"date"`
				TokensByModel map[string]int64 `json:"tokensByModel"`
			}{day, map[string]int64{
				"claude-opus-4-8": perDay(i),
				"claude-fable-5":  perDay(i), // per-model counts are summed
			}})
		}
		return s
	}
	latestVs := func(latest, prior int64) statsCache {
		return build(14, func(i int) int64 {
			if i == 0 {
				return latest
			}
			return prior
		})
	}

	// Latest day 2×200k = 400k against a 2×100k = 200k daily average.
	if got := stripANSI(renderTokenTrend(latestVs(200_000, 100_000), now)); got != "daily 400.0k ▲100%" {
		t.Errorf("got %q, want %q", got, "daily 400.0k ▲100%")
	}
	// A quiet day reads as a drop.
	if got := stripANSI(renderTokenTrend(latestVs(50_000, 200_000), now)); got != "daily 100.0k ▼75%" {
		t.Errorf("got %q, want %q", got, "daily 100.0k ▼75%")
	}
	// Flat usage is neither up nor down.
	if got := stripANSI(renderTokenTrend(latestVs(100_000, 100_000), now)); got != "daily 200.0k ▲0%" {
		t.Errorf("got %q, want %q", got, "daily 200.0k ▲0%")
	}

	// Eight days is the minimum: the reported day plus a full week behind it.
	if got := renderTokenTrend(build(8, func(int) int64 { return 100_000 }), now); got == "" {
		t.Error("eight days of history should be enough to average over")
	}
	// Seven isn't — the missing days would count as zero-token days and drag
	// the average down.
	if got := renderTokenTrend(build(7, func(int) int64 { return 100_000 }), now); got != "" {
		t.Errorf("a short history should say nothing, got %q", stripANSI(got))
	}

	// Newest date well before now → stale, say nothing.
	if got := renderTokenTrend(build(14, func(int) int64 { return 5000 }), now.AddDate(0, 0, 30)); got != "" {
		t.Errorf("stale cache should render nothing, got %q", stripANSI(got))
	}
	// No usage on the reported day, and nothing to divide by.
	if got := renderTokenTrend(latestVs(0, 100_000), now); got != "" {
		t.Errorf("a zero day should render nothing, got %q", stripANSI(got))
	}
	if got := renderTokenTrend(latestVs(100_000, 0), now); got != "" {
		t.Errorf("an empty baseline should render nothing, got %q", stripANSI(got))
	}
	if got := renderTokenTrend(statsCache{}, now); got != "" {
		t.Errorf("empty cache should render nothing, got %q", stripANSI(got))
	}
}

func stripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == 0x1b {
			for i < len(s) && s[i] != 'm' {
				i++
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
