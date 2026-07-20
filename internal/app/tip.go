package app

import (
	"fmt"
	"time"
)

// tipContext is everything the third line needs to pick what to say.
type tipContext struct {
	ctxPct   int // context window used
	limitPct int // highest plan bucket: 5h, 7d, or a per-model quota
	weekPct  int // 7-day bucket
	compact  compactState
	plugin   string
}

func tipLine(t tipContext) string {
	if t.compact.userSet && t.compact.pct > 0 && t.ctxPct >= t.compact.pct {
		return dimGray + "auto-compact imminent" + reset + " " + dim + "·" + reset + " " +
			dimGray + "/compact" + reset + " " + dim + "now" + reset + " " + dim + "·" + reset + " " +
			dimGray + "/clear" + reset + " " + dim + "reset" + reset
	}
	if t.limitPct >= 80 {
		return dimGray + "rate limit high" + reset + " " + dim + "·" + reset + " " +
			dimGray + "/usage-credits" + reset + " " + dim + "raise your limit" + reset + " " + dim + "·" + reset + " " +
			dimGray + "/model" + reset + " " + dim + "switch for more runway" + reset
	}
	if t.plugin != "" {
		return t.plugin
	}
	// Rare and actionable, so it outranks the tip rotation — but not the
	// warnings above, and not a user's own plugin line.
	if tag := availableUpdate(6 * time.Hour); tag != "" {
		return rainbow("✦ claude-gisx "+tag+" available") + " " + dim + "·" + reset + " " +
			dimGray + "claude-gisx update" + reset
	}
	if t.ctxPct >= 70 {
		return dimGray + "/compact" + reset + " " + dim + "free context" + reset + " " + dim + "·" + reset + " " +
			dimGray + "/clear" + reset + " " + dim + "reset session" + reset
	}
	if t.ctxPct >= 40 {
		return dimGray + "/compact" + reset + " " + dim + "free context" + reset + " " + dim + "·" + reset + " " +
			dimGray + "shift+tab" + reset + " " + dim + "interrupt" + reset + " " + dim + "·" + reset + " " +
			dimGray + "esc esc" + reset + " " + dim + "cancel" + reset
	}
	pool := shortcutTips()
	pool = append(pool, whatsNew()...)
	if p := usagePromo(t.weekPct); p != "" {
		pool = append(pool, p)
	}
	if p := weeklyTrend(); p != "" {
		pool = append(pool, p)
	}
	return pool[int(time.Now().Unix())%len(pool)]
}

func shortcutTips() []string {
	return []string{
		dimGray + "shift+tab" + reset + " " + dim + "interrupt" + reset + " " + dim + "·" + reset + " " +
			dimGray + "/compact" + reset + " " + dim + "free context" + reset + " " + dim + "·" + reset + " " +
			dimGray + "esc esc" + reset + " " + dim + "cancel" + reset,
		dimGray + "ctrl+r" + reset + " " + dim + "retry" + reset + " " + dim + "·" + reset + " " +
			dimGray + "#" + reset + " " + dim + "add files" + reset + " " + dim + "·" + reset + " " +
			dimGray + "/cost" + reset + " " + dim + "session cost" + reset,
		dimGray + "/init" + reset + " " + dim + "setup CLAUDE.md" + reset + " " + dim + "·" + reset + " " +
			dimGray + "/review" + reset + " " + dim + "review changes" + reset + " " + dim + "·" + reset + " " +
			dimGray + "/help" + reset + " " + dim + "commands" + reset,
		dimGray + "/model" + reset + " " + dim + "switch model" + reset + " " + dim + "·" + reset + " " +
			dimGray + "/vim" + reset + " " + dim + "vim mode" + reset + " " + dim + "·" + reset + " " +
			dimGray + "/config" + reset + " " + dim + "settings" + reset,
	}
}

// usagePromo nudges toward more headroom once the week is half spent — the
// weekly bucket is the one you can actually raise.
func usagePromo(weekPct int) string {
	if weekPct < 50 {
		return ""
	}
	return dimGray + fmt.Sprintf("weekly %d%% used", weekPct) + reset + " " + dim + "·" + reset + " " +
		dimGray + "/usage-credits" + reset + " " + dim + "raise your weekly limit" + reset
}
