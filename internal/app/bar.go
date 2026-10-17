package app

import "strings"

const barWidth = 10

// Left-aligned eighth blocks, index = eighths filled. A 10-cell bar in whole
// cells moves in 10% steps; eighths bring that to 1.25%.
var partials = [8]string{"", "▏", "▎", "▍", "▌", "▋", "▊", "▉"}

// ctxBar is the one progress bar on the statusline: everything else is a
// number, so the eye goes straight to how much context is left. The fill is
// one solid color, the same as the percentage beside it, and the whole bar
// changes color at once when usage crosses a threshold (see pctColor).
func ctxBar(pctUsed int) string {
	p := clampi(pctUsed, 0, 100)
	eighths := (p * barWidth * 8) / 100
	full, part := eighths/8, eighths%8
	cells := full
	var b strings.Builder
	b.WriteString(pctColor(p) + strings.Repeat("█", full))
	if part > 0 {
		b.WriteString(partials[part])
		cells++
	}
	b.WriteString(dimGray + strings.Repeat("░", barWidth-cells) + reset)
	return b.String()
}

// ctxBarBy draws the same bar with its fill split by category: as long as
// ctxBar's, each category's share of it in that category's color. Cells go
// whole to one category, so the eighth-cell edge takes the color of the last
// one, which in /context's order is the conversation. Shares are strictly
// proportional: on ten cells a floor of one per category would hand a few
// hundred tokens of agents the same cell as 200k of messages, so the small
// ones are left to the key beneath.
func ctxBarBy(pctUsed int, segs []ctxSeg) string {
	if len(segs) == 0 {
		return ctxBar(pctUsed)
	}
	p := clampi(pctUsed, 0, 100)
	eighths := (p * barWidth * 8) / 100
	full, part := eighths/8, eighths%8
	sizes := make([]int64, len(segs))
	for i, s := range segs {
		sizes[i] = s.tokens
	}
	cells := apportion(sizes, full, 0)
	var b strings.Builder
	last := ""
	paint := func(col, glyph string) {
		if col != last {
			b.WriteString(col)
			last = col
		}
		b.WriteString(glyph)
	}
	for i, s := range segs {
		for range cells[i] {
			paint(s.color, "█")
		}
	}
	drawn := full
	if part > 0 {
		paint(segs[len(segs)-1].color, partials[part])
		drawn++
	}
	paint(dimGray, strings.Repeat("░", barWidth-drawn))
	b.WriteString(reset)
	return b.String()
}
