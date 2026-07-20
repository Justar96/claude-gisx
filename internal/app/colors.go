package app

import "fmt"

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
