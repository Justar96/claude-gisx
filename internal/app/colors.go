package app

import (
	"fmt"
	"strings"
	"time"
)

// 24-bit ANSI escapes; produces empty strings when stdout isn't a terminal.
var (
	reset   = "\x1b[0m"
	bold    = "\x1b[1m"
	dim     = "\x1b[2m"
	red     = rgb(255, 85, 85)
	green   = rgb(0, 175, 80)
	yellow  = rgb(255, 200, 50)
	orange  = rgb(255, 150, 50)
	cyan    = rgb(86, 182, 194)
	blue    = rgb(0, 153, 255)
	magenta = rgb(180, 140, 255)
	white   = rgb(220, 220, 220)
	dimGray = rgb(120, 120, 120)
	sep     = dim + " · " + reset
)

func rgb(r, g, b int) string {
	return fmt.Sprintf("\x1b[38;2;%d;%d;%dm", r, g, b)
}

// disableColor blanks every escape so output is plain text.
//
// Deliberately not driven by a bare isatty check: Claude Code runs this binary
// with stdout on a pipe and renders the escapes itself, so testing the
// terminal on the statusline path would strip the color from the one place it
// matters most. Only the subcommands consult the terminal — see Run. NO_COLOR
// is honoured everywhere, because that one is the user asking.
func disableColor() {
	reset, bold, dim = "", "", ""
	red, green, yellow, orange = "", "", "", ""
	cyan, blue, magenta, white, dimGray = "", "", "", "", ""
	// Both of these were composed from the vars above at init, so they hold
	// stale escapes until rebuilt by hand.
	sep = " · "
	okMark, failMark, dotMark = "✓", "✗", "·"
}

func pctColor(p int) string {
	switch {
	case p >= 90:
		return red
	case p >= 70:
		return yellow
	case p >= 50:
		return orange
	case p >= 20:
		return cyan
	default:
		return green
	}
}

// pinkTint shimmers a red-to-pink ramp across the string, one step per
// character. The ramp runs back down so the colors never jump from the
// lightest shade to the darkest; the phase advances with the clock, so the
// tint drifts between renders.
func pinkTint(s string) string {
	palette := []string{
		rgb(235, 60, 90), rgb(255, 90, 120), rgb(255, 125, 155),
		rgb(255, 160, 190), rgb(255, 125, 155), rgb(255, 90, 120),
	}
	shift := int(time.Now().Unix())
	var b strings.Builder
	for i, r := range s {
		if r == ' ' {
			b.WriteRune(r)
			continue
		}
		b.WriteString(palette[(i+shift)%len(palette)])
		b.WriteRune(r)
	}
	b.WriteString(reset)
	return b.String()
}

func effortColor(level string) string {
	switch level {
	case "max", "xhigh":
		return red
	case "high":
		return cyan
	case "low":
		return dimGray
	default:
		return white
	}
}

// Cache hit rates live in a much narrower band than the quota percentages
// remainingColor grades — a healthy session sits in the 90s and anything
// below ~75% means the prefix keeps getting invalidated. On the quota scale
// that whole range is one flat green, so it gets its own thresholds.
func cacheColor(p int) string {
	switch {
	case p >= 90:
		return green
	case p >= 75:
		return cyan
	case p >= 50:
		return yellow
	default:
		return red
	}
}

func remainingColor(p int) string {
	switch {
	case p >= 70:
		return green
	case p >= 30:
		return yellow
	default:
		return red
	}
}
