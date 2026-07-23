package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Claude Code writes this when you open /usage or the stats view, so it can
// lag a few days — the trend is reported against its own newest date.
func statsPath() string { return filepath.Join(claudeDir(), "stats-cache.json") }

type statsCache struct {
	DailyModelTokens []struct {
		Date          string           `json:"date"`
		TokensByModel map[string]int64 `json:"tokensByModel"`
	} `json:"dailyModelTokens"`
}

// Holds the computed numbers, never the painted string: caching rendered ANSI
// would replay whatever colors were in effect when it was written, so a later
// NO_COLOR run would still emit escapes from the cache.
type trendCache struct {
	Stamp  string `json:"stamp"`
	Tokens int64  `json:"tokens"` // 0 means "nothing worth showing"
	Delta  int    `json:"delta"`
}

func trendCachePath() string { return filepath.Join(lineCacheDir(), "statusline-trend.json") }

// The trend is derived from a file Claude Code rewrites at most once a day,
// but it was re-parsing ~30KB of JSON on every render. Memoize it against the
// source's mtime and size so the steady-state cost is a stat plus a tiny read.
// The current date is part of the key too: the result also depends on how old
// the newest sample is, so a cache keyed on the file alone would keep showing
// a trend that should have aged out.
func tokenTrend() string {
	st, err := os.Stat(statsPath())
	if err != nil {
		return ""
	}
	now := time.Now()
	stamp := fmt.Sprintf("%d/%d/%s", st.ModTime().UnixNano(), st.Size(), now.Format("2006-01-02"))
	if raw, err := os.ReadFile(trendCachePath()); err == nil {
		var c trendCache
		if json.Unmarshal(raw, &c) == nil && c.Stamp == stamp {
			return paintTokenTrend(c.Tokens, c.Delta)
		}
	}

	var tokens int64
	delta := 0
	if raw, err := os.ReadFile(statsPath()); err == nil {
		var s statsCache
		if json.Unmarshal(raw, &s) == nil {
			tokens, delta = computeTokenTrend(s, now)
		}
	}
	if raw, err := json.Marshal(trendCache{Stamp: stamp, Tokens: tokens, Delta: delta}); err == nil {
		_ = writeFileAtomic(trendCachePath(), raw)
	}
	return paintTokenTrend(tokens, delta)
}

const (
	staleAfterDays = 10
	trendWindow    = 7 // days of history the daily average is drawn from
)

// renderTokenTrend reports the latest day on record against the daily average
// of the week before it — "tokens 924.2k ▼68%" — so it moves the day you burn
// more, instead of waiting a week for a week-over-week figure to budge.
//
// Everything is measured relative to the file's own newest date rather than
// the wall clock. Claude Code only rewrites this file when you open /usage, so
// the newest day it holds is the newest day we can speak to; calling it
// "today" when the file lagged three days would be a lie.
func renderTokenTrend(s statsCache, now time.Time) string {
	return paintTokenTrend(computeTokenTrend(s, now))
}

// paintTokenTrend is kept apart from the arithmetic so the colors are chosen
// at render time — see trendCache for why that matters.
func paintTokenTrend(tokens int64, delta int) string {
	if tokens == 0 {
		return ""
	}
	arrow, col := "▲", green
	if delta < 0 {
		arrow, col, delta = "▼", dimGray, -delta
	}
	return dimGray + "tokens" + reset + " " + white + fmtTokens(tokens) + reset +
		" " + col + fmt.Sprintf("%s%d%%", arrow, delta) + reset
}

// computeTokenTrend returns the reported day's tokens and its percentage
// change, or (0, 0) when there isn't enough recent data to say anything true.
func computeTokenTrend(s statsCache, now time.Time) (int64, int) {
	days := make(map[string]int64, len(s.DailyModelTokens))
	dates := make([]string, 0, len(s.DailyModelTokens))
	for _, d := range s.DailyModelTokens {
		if d.Date == "" {
			continue
		}
		total := int64(0)
		for _, n := range d.TokensByModel {
			total += n
		}
		days[d.Date] = total
		dates = append(dates, d.Date)
	}
	if len(dates) == 0 {
		return 0, 0
	}
	sort.Strings(dates)
	const layout = "2006-01-02"
	newest, err := time.Parse(layout, dates[len(dates)-1])
	if err != nil {
		return 0, 0
	}
	oldest, err := time.Parse(layout, dates[0])
	if err != nil {
		return 0, 0
	}
	// A day with no usage is simply absent from the file, so a short history
	// would read as a run of zero-token days and drag the average down.
	// Require the window to be genuinely covered before averaging over it.
	if oldest.After(newest.AddDate(0, 0, -trendWindow)) {
		return 0, 0
	}
	if now.Sub(newest) > staleAfterDays*24*time.Hour {
		return 0, 0
	}

	latest := days[newest.Format(layout)]
	if latest == 0 {
		return 0, 0
	}
	sum := int64(0)
	for i := 1; i <= trendWindow; i++ {
		sum += days[newest.AddDate(0, 0, -i).Format(layout)]
	}
	if sum == 0 {
		return 0, 0
	}
	avg := sum / trendWindow
	return latest, int((latest - avg) * 100 / avg)
}

func fmtTokens(n int64) string {
	switch {
	case n >= 1_000_000_000:
		return fmt.Sprintf("%.1fB", float64(n)/1e9)
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1e6)
	case n >= 1000:
		return fmt.Sprintf("%.1fk", float64(n)/1e3)
	default:
		return fmt.Sprintf("%d", n)
	}
}
