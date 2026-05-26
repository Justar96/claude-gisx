package main

import (
	"fmt"
	"time"
)

func fmtRemaining(epoch int64) string {
	if epoch <= 0 {
		return ""
	}
	diff := epoch - time.Now().Unix()
	if diff <= 0 {
		return "now"
	}
	d := diff / 86400
	h := (diff % 86400) / 3600
	m := (diff % 3600) / 60
	switch {
	case d > 0 && h > 0:
		return fmt.Sprintf("%dd %dh", d, h)
	case d > 0:
		return fmt.Sprintf("%dd", d)
	case h > 0 && m > 0:
		return fmt.Sprintf("%dh %dm", h, m)
	case h > 0:
		return fmt.Sprintf("%dh", h)
	case m > 0:
		return fmt.Sprintf("%dm", m)
	default:
		return fmt.Sprintf("%ds", diff)
	}
}

func fmtDuration(ms int64) string {
	if ms <= 0 {
		return ""
	}
	sec := ms / 1000
	switch {
	case sec >= 3600:
		return fmt.Sprintf("%dh%dm", sec/3600, (sec%3600)/60)
	case sec >= 60:
		return fmt.Sprintf("%dm", sec/60)
	default:
		return fmt.Sprintf("%ds", sec)
	}
}

func fmtCtxLabel(size int) string {
	switch {
	case size >= 1_000_000:
		return fmt.Sprintf("%dM", size/1_000_000)
	case size >= 1000:
		return fmt.Sprintf("%dk", size/1000)
	default:
		return fmt.Sprintf("%d", size)
	}
}

func clampi(n, lo, hi int) int {
	if n < lo {
		return lo
	}
	if n > hi {
		return hi
	}
	return n
}
