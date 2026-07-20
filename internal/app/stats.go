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

// weeklyTrend renders "this week 4.2M tokens · ▲18% vs last week", or "" when
// there isn't enough recent data to say anything true.
func weeklyTrend() string {
	raw, err := os.ReadFile(statsPath())
	if err != nil {
		return ""
	}
	var s statsCache
	if err := json.Unmarshal(raw, &s); err != nil {
		return ""
	}
	return renderWeeklyTrend(s, time.Now())
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
	return dimGray + "this week " + fmtTokens(this) + " tokens" + reset + " " + dim + "·" + reset + " " +
		col + fmt.Sprintf("%s%d%%", arrow, delta) + reset + " " + dim + "vs last week" + reset
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
