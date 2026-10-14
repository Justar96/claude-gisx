package app

import (
	"strings"
	"testing"
)

// countFilled counts the "█" runes in the bar — independent of color escapes.
func countFilled(s string) int {
	return strings.Count(s, "█")
}

func TestCtxBarFilledCount(t *testing.T) {
	cases := []struct {
		pct  int
		want int
	}{
		{0, 0},
		{6, 0},  // 6 * 15 / 100 = 0
		{7, 1},  // 7 * 15 / 100 = 1
		{50, 7}, // 50 * 15 / 100 = 7
		{100, 15},
		{200, 15}, // clamped
		{-10, 0},  // clamped
	}
	for _, c := range cases {
		got := countFilled(ctxBar(c.pct))
		if got != c.want {
			t.Errorf("ctxBar(%d) filled=%d, want %d", c.pct, got, c.want)
		}
	}
}

func TestCtxBarPartialCell(t *testing.T) {
	// 50% of 15 cells is 7.5: seven full cells, then a half block.
	if s := ansiRe.ReplaceAllString(ctxBar(50), ""); s != "███████▌░░░░░░░" {
		t.Errorf("ctxBar(50) should end its fill in a half block; got %q", s)
	}
	// Every cell is drawn exactly once, whatever the fill.
	for _, p := range []int{0, 3, 42, 50, 99, 100} {
		if n := len([]rune(ansiRe.ReplaceAllString(ctxBar(p), ""))); n != barWidth {
			t.Errorf("ctxBar(%d) is %d cells wide, want %d", p, n, barWidth)
		}
	}
}

func TestCtxBarResets(t *testing.T) {
	s := ctxBar(50)
	if !strings.HasSuffix(s, reset) {
		t.Errorf("ctxBar should end with reset escape; got: %q", s)
	}
}

func TestPctColor(t *testing.T) {
	cases := []struct {
		p    int
		want string
	}{
		{0, green},
		{19, green},
		{20, cyan},
		{49, cyan},
		{50, orange},
		{69, orange},
		{70, yellow},
		{89, yellow},
		{90, red},
		{100, red},
	}
	for _, c := range cases {
		if got := pctColor(c.p); got != c.want {
			t.Errorf("pctColor(%d) mismatch", c.p)
		}
	}
}

func TestRemainingColor(t *testing.T) {
	cases := []struct {
		p    int
		want string
	}{
		{0, red},
		{29, red},
		{30, yellow},
		{69, yellow},
		{70, green},
		{100, green},
	}
	for _, c := range cases {
		if got := remainingColor(c.p); got != c.want {
			t.Errorf("remainingColor(%d) mismatch", c.p)
		}
	}
}
