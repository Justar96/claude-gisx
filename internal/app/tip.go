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

// A warning holds the line for two renders out of every three. Both warnings
// stay true for a long time — past the compact threshold until you compact, a
// spent quota until it resets — so returning them outright would mean the
// rotation never runs at all.
const warnHoldOf = 3

func tipLine(t tipContext) string {
	tick := time.Now().Unix()
	if w := warningLine(t); w != "" {
		if tick%warnHoldOf != warnHoldOf-1 {
			return w
		}
		// Divide so the freed ticks still walk the whole pool rather than
		// landing on the same few entries.
		tick /= warnHoldOf
	}
	if t.plugin != "" {
		return t.plugin
	}
	// Rare and actionable, so it outranks the rotation — but not the warnings
	// above, and not a user's own plugin line.
	if tag := availableUpdate(6 * time.Hour); tag != "" {
		return pinkTint("✦ claude-gisx "+tag+" available") + " " + dim + "·" + reset + " " +
			dimGray + "claude-gisx update" + reset
	}

	pool := whatsNew()
	if p := usagePromo(t.weekPct); p != "" {
		pool = append(pool, p)
	}
	if len(pool) == 0 {
		return ""
	}
	return pool[int(tick)%len(pool)]
}

func warningLine(t tipContext) string {
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
	return ""
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
