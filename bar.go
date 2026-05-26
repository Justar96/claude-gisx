package main

import "strings"

const barWidth = 15

func ctxBar(pctUsed int) string {
	p := clampi(pctUsed, 0, 100)
	filled := (p * barWidth) / 100
	var b strings.Builder
	for i := 0; i < barWidth; i++ {
		if i < filled {
			pos := ((i + 1) * 100) / barWidth
			var col string
			switch {
			case pos <= 20:
				col = green
			case pos <= 47:
				col = cyan
			case pos <= 67:
				col = orange
			case pos <= 87:
				col = yellow
			default:
				col = red
			}
			b.WriteString(col)
			b.WriteString("█")
		} else {
			b.WriteString(dimGray)
			b.WriteString("░")
		}
	}
	b.WriteString(reset)
	return b.String()
}
