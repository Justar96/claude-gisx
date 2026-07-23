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

// weeklyTrend renders a compact "4.2M ▲18%" for the usage line — this week's
// tokens and the change against the previous week. Empty when there isn't
// enough recent data to say anything true.
type trendCache struct {
	Stamp    string `json:"stamp"`
	Rendered string `json:"rendered"`
}

func trendCachePath() string { return filepath.Join(lineCacheDir(), "statusline-trend.json") }

// The trend is derived from a file Claude Code rewrites at most once a day,
// but it was re-parsing ~30KB of JSON on every render. Memoize it against the
// source's mtime and size so the steady-state cost is a stat plus a tiny read.
// The current date is part of the key too: the result also depends on how old
// the newest sample is, so a cache keyed on the file alone would keep showing
// a trend that should have aged out.
func weeklyTrend() string {
	st, err := os.Stat(statsPath())
	if err != nil {
		return ""
	}
	now := time.Now()
	stamp := fmt.Sprintf("%d/%d/%s", st.ModTime().UnixNano(), st.Size(), now.Format("2006-01-02"))
	if raw, err := os.ReadFile(trendCachePath()); err == nil {
		var c trendCache
		if json.Unmarshal(raw, &c) == nil && c.Stamp == stamp {
			return c.Rendered
		}
	}

	rendered := ""
	if raw, err := os.ReadFile(statsPath()); err == nil {
		var s statsCache
		if json.Unmarshal(raw, &s) == nil {
			rendered = renderWeeklyTrend(s, now)
		}
	}
	if raw, err := json.Marshal(trendCache{Stamp: stamp, Rendered: rendered}); err == nil {
		_ = writeFileAtomic(trendCachePath(), raw)
	}
	return rendered
}

const staleAfterDays = 10

func renderWeeklyTrend(s statsCache, now time.Time) string {
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
	if len(dates) < 14 {
		return ""
	}
	sort.Strings(dates)
	newest, err := time.Parse("2006-01-02", dates[len(dates)-1])
	if err != nil {
		return ""
	}
	if now.Sub(newest) > staleAfterDays*24*time.Hour {
		return ""
	}
	this, prev := int64(0), int64(0)
	for i := 0; i < 7; i++ {
		this += days[newest.AddDate(0, 0, -i).Format("2006-01-02")]
		prev += days[newest.AddDate(0, 0, -i-7).Format("2006-01-02")]
	}
	if this == 0 || prev == 0 {
		return ""
	}
	delta := (this - prev) * 100 / prev
	arrow, col := "▲", green
	if delta < 0 {
		arrow, col, delta = "▼", dimGray, -delta
	}
	return white + fmtTokens(this) + reset + " " + col + fmt.Sprintf("%s%d%%", arrow, delta) + reset
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
