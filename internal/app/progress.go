package app

import (
	"fmt"
	"io"
	"strings"
	"time"
)

const progWidth = 15

// A download fills toward "good", which is the opposite of what the context
// bar means, so it gets its own colors rather than reusing pctColor — a
// nearly-finished download painted red would read as a problem.
func downloadBar(pct int) string {
	pct = clampi(pct, 0, 100)
	filled := pct * progWidth / 100
	var b strings.Builder
	b.WriteString(cyan)
	for i := 0; i < progWidth; i++ {
		if i == filled {
			b.WriteString(dimGray)
		}
		if i < filled {
			b.WriteString("█")
		} else {
			b.WriteString("░")
		}
	}
	b.WriteString(reset)
	return b.String()
}

// progressReader repaints a bar in place as the body streams. It is only
// wired up when stdout is a terminal: carriage-return rewrites turn into a
// wall of noise in a redirected log.
type progressReader struct {
	r     io.Reader
	total int64 // 0 when the server sent no Content-Length
	read  int64
	last  time.Time
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	p.read += int64(n)
	// Repainting on every chunk is wasted work and makes the bar flicker;
	// an error or EOF always paints so the final frame shows 100%.
	if err != nil || time.Since(p.last) >= 80*time.Millisecond {
		p.last = time.Now()
		p.paint()
	}
	return n, err
}

func (p *progressReader) paint() {
	size := fmtTokens(p.read)
	if p.total <= 0 {
		// No Content-Length, so there's no percentage to be honest about —
		// show the bytes climbing and nothing more.
		fmt.Printf("\r  %s↓%s %s%s%s", cyan, reset, dimGray, size, reset)
		return
	}
	pct := clampi(int(p.read*100/p.total), 0, 100)
	fmt.Printf("\r  %s↓%s %s %s%3d%%%s  %s%s/%s%s",
		cyan, reset, downloadBar(pct), white, pct, reset,
		dimGray, size, fmtTokens(p.total), reset)
}

// clear wipes the bar so the caller can print a settled line in its place.
// Spaces rather than an erase-line escape, so this still behaves under
// NO_COLOR and on a console that only handles the basics.
func (p *progressReader) clear() {
	fmt.Print("\r" + strings.Repeat(" ", 56) + "\r")
}
