package app

import "strings"

const barWidth = 15

// Left-aligned eighth blocks, index = eighths filled. A 15-cell bar in whole
// cells moves in ~6.7% steps; eighths bring that under 1%.
var partials = [8]string{"", "▏", "▎", "▍", "▌", "▋", "▊", "▉"}

// barColor grades a cell by where it sits along the bar, so the fill shifts
// from green to red as it grows.
func barColor(cell int) string {
	pos := ((cell + 1) * 100) / barWidth
	switch {
	case pos <= 20:
		return green
	case pos <= 47:
		return cyan
	case pos <= 67:
		return orange
	case pos <= 87:
		return yellow
	default:
		return red
	}
}

// ctxBar is the one progress bar on the statusline: everything else is a
// number, so the eye goes straight to how much context is left.
func ctxBar(pctUsed int) string {
	p := clampi(pctUsed, 0, 100)
	eighths := (p * barWidth * 8) / 100
	full, part := eighths/8, eighths%8
	var b strings.Builder
	// An escape only where the color changes: the statusline redraws on every
	// message, and fifteen identical escapes in a row are pure noise.
	last := ""
	paint := func(col, glyph string) {
		if col != last {
			b.WriteString(col)
			last = col
		}
		b.WriteString(glyph)
	}
	for i := 0; i < barWidth; i++ {
		switch {
		case i < full:
			paint(barColor(i), "█")
		case i == full && part > 0:
			paint(barColor(i), partials[part])
		default:
			paint(dimGray, "░")
		}
	}
	b.WriteString(reset)
	return b.String()
}
