package app

import (
	"strings"
	"testing"
	"time"
)

const changelogSample = `# Changelog

## 2.1.215

- Claude no longer runs the ` + "`/verify`" + ` skill on its own
- Fixed a crash when a GrowthBook feature evaluates to null
- Added the EndConversation tool — see https://www.anthropic.com/research/end-subset-conversations
- Added reasoning effort to the ` + "`subagentStatusLine`" + ` payload

## 2.1.212

- ` + "`/fork`" + ` now copies your conversation into a new background session
`

func TestParseChangelog(t *testing.T) {
	got := parseChangelog(strings.NewReader(changelogSample))
	want := []changelogEntry{
		{"2.1.215", "Claude no longer runs the /verify skill on its own"},
		{"2.1.215", "Added reasoning effort to the subagentStatusLine payload"},
		{"2.1.212", "/fork now copies your conversation into a new background session"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d entries, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("entry %d: got %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestCleanEntry(t *testing.T) {
	long := strings.Repeat("word ", 40)
	cases := []struct{ in, want string }{
		{"Fixed a thing", ""},
		{"Added a link https://example.com", ""},
		{"Added `foo` support", "Added foo support"},
		{"Nosplit" + strings.Repeat("x", 200), ""}, // no word boundary to cut at
	}
	for _, c := range cases {
		if got := cleanEntry(c.in); got != c.want {
			t.Errorf("cleanEntry(%.30q) = %q, want %q", c.in, got, c.want)
		}
	}
	got := cleanEntry(long)
	if len([]rune(got)) > maxEntryLen+1 || !strings.HasSuffix(got, "…") {
		t.Errorf("long entry not truncated cleanly: %q", got)
	}
}

func TestRenderWeeklyTrend(t *testing.T) {
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	build := func(perDay func(i int) int64) statsCache {
		var s statsCache
		for i := 0; i < 14; i++ {
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

	// 7 recent days at 2×200k = 2.8M vs 7 prior at 2×100k = 1.4M → +100%
	got := renderWeeklyTrend(build(func(i int) int64 {
		if i < 7 {
			return 200_000
		}
		return 100_000
	}), now)
	if stripANSI(got) != "2.8M ▲100%" {
		t.Errorf("got %q, want %q", stripANSI(got), "2.8M ▲100%")
	}

	// Newest date well before now → stale, say nothing.
	if got := renderWeeklyTrend(build(func(int) int64 { return 5000 }), now.AddDate(0, 0, 30)); got != "" {
		t.Errorf("stale cache should render nothing, got %q", stripANSI(got))
	}

	// Not enough history.
	if got := renderWeeklyTrend(statsCache{}, now); got != "" {
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
