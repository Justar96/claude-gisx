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
		{9, 0},  // 9% of 10 cells is 0.9: a partial, no full cell
		{10, 1}, // 10% of 10 cells is 1
		{50, 5}, // 50% of 10 cells is 5
		{100, 10},
		{200, 10}, // clamped
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
	// 55% of 10 cells is 5.5: five full cells, then a half block.
	if s := ansiRe.ReplaceAllString(ctxBar(55), ""); s != "█████▌░░░░" {
		t.Errorf("ctxBar(55) should end its fill in a half block; got %q", s)
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

// The fill is one color for the whole bar, chosen by the total, not by
// where each cell sits.
func TestCtxBarSolidColor(t *testing.T) {
	for _, p := range []int{10, 35, 60, 80, 95} {
		s := ctxBar(p)
		if !strings.HasPrefix(s, pctColor(p)) {
			t.Errorf("ctxBar(%d) should open with pctColor(%d)", p, p)
		}
		for _, col := range []string{green, cyan, orange, yellow, red} {
			if col != pctColor(p) && strings.Contains(s, col) {
				t.Errorf("ctxBar(%d) mixes in a second fill color", p)
			}
		}
	}
}

func TestCtxBarBy(t *testing.T) {
	segs := []ctxSeg{{"system", 10, blue}, {"tools", 30, cyan}, {"messages", 60, magenta}}
	// No breakdown: the plain single-color bar.
	if ctxBarBy(40, nil) != ctxBar(40) {
		t.Error("without categories the bar should be ctxBar's")
	}
	// 55% of 10 cells: five whole cells split 1/1/3 by share (largest
	// remainder), then a half block in the last category's color.
	got := ctxBarBy(55, segs)
	if plain := ansiRe.ReplaceAllString(got, ""); plain != "█████▌░░░░" {
		t.Errorf("ctxBarBy(55) = %q, want the same length as ctxBar", plain)
	}
	want := blue + "█" + cyan + "█" + magenta + "███▌" + dimGray + "░░░░" + reset
	if got != want {
		t.Errorf("ctxBarBy(55) colors:\n got %q\nwant %q", got, want)
	}
	// No floor: a sliver of a category doesn't take a whole cell.
	tiny := []ctxSeg{{"agents", 1, blue}, {"messages", 999, magenta}}
	if got := ctxBarBy(50, tiny); strings.Contains(got, blue) {
		t.Errorf("a 0.1%% category drew a cell: %q", ansiRe.ReplaceAllString(got, ""))
	}
	for _, p := range []int{0, 3, 42, 99, 100, 150} {
		if n := len([]rune(ansiRe.ReplaceAllString(ctxBarBy(p, segs), ""))); n != barWidth {
			t.Errorf("ctxBarBy(%d) is %d cells wide, want %d", p, n, barWidth)
		}
	}
}
