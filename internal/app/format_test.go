package app

import (
	"testing"
	"time"
)

func TestFmtCtxLabel(t *testing.T) {
	cases := []struct {
		in   int
		want string
	}{
		{0, "0"},
		{999, "999"},
		{1000, "1k"},
		{200_000, "200k"},
		{999_999, "999k"},
		{1_000_000, "1M"},
		{2_000_000, "2M"},
	}
	for _, c := range cases {
		if got := fmtCtxLabel(c.in); got != c.want {
			t.Errorf("fmtCtxLabel(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestFmtDuration(t *testing.T) {
	cases := []struct {
		ms   int64
		want string
	}{
		{0, ""},
		{-1, ""},
		{500, "0s"},
		{1_000, "1s"},
		{59_000, "59s"},
		{60_000, "1m"},
		{3_599_000, "59m"},
		{3_600_000, "1h0m"},
		{3_660_000, "1h1m"},
		{7_320_000, "2h2m"},
	}
	for _, c := range cases {
		if got := fmtDuration(c.ms); got != c.want {
			t.Errorf("fmtDuration(%d) = %q, want %q", c.ms, got, c.want)
		}
	}
}

func TestFmtRemaining(t *testing.T) {
	if got := fmtRemaining(0); got != "" {
		t.Errorf("fmtRemaining(0) = %q, want empty", got)
	}
	if got := fmtRemaining(-100); got != "" {
		t.Errorf("fmtRemaining(-100) = %q, want empty", got)
	}
	past := time.Now().Unix() - 10
	if got := fmtRemaining(past); got != "now" {
		t.Errorf("fmtRemaining(past) = %q, want %q", got, "now")
	}

	cases := []struct {
		offset time.Duration
		want   string
	}{
		{30 * time.Second, "30s"},
		{5 * time.Minute, "5m"},
		{1 * time.Hour, "1h"},
		{1*time.Hour + 30*time.Minute, "1h 30m"},
		{25 * time.Hour, "1d 1h"},
		{49 * time.Hour, "2d 1h"},
		{48 * time.Hour, "2d"},
	}
	for _, c := range cases {
		epoch := time.Now().Add(c.offset).Unix()
		if got := fmtRemaining(epoch); got != c.want {
			t.Errorf("fmtRemaining(now+%s) = %q, want %q", c.offset, got, c.want)
		}
	}
}

func TestClampi(t *testing.T) {
	cases := []struct {
		n, lo, hi, want int
	}{
		{5, 0, 10, 5},
		{-1, 0, 10, 0},
		{11, 0, 10, 10},
		{0, 0, 10, 0},
		{10, 0, 10, 10},
	}
	for _, c := range cases {
		if got := clampi(c.n, c.lo, c.hi); got != c.want {
			t.Errorf("clampi(%d, %d, %d) = %d, want %d", c.n, c.lo, c.hi, got, c.want)
		}
	}
}
