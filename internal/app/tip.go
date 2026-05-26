package app

import "time"

func tipLine(pctUsed, fivePct, weekPct int, cs compactState, plugin string) string {
	if cs.userSet && cs.pct > 0 && pctUsed >= cs.pct {
		return dimGray + "auto-compact imminent" + reset + " " + dim + "·" + reset + " " +
			dimGray + "/compact" + reset + " " + dim + "now" + reset + " " + dim + "·" + reset + " " +
			dimGray + "/clear" + reset + " " + dim + "reset" + reset
	}
	if fivePct >= 80 || weekPct >= 80 {
		return dimGray + "rate limit high" + reset + " " + dim + "·" + reset + " " +
			dimGray + "consider pausing or switching models" + reset
	}
	if plugin != "" {
		return plugin
	}
	if pctUsed >= 70 {
		return dimGray + "/compact" + reset + " " + dim + "free context" + reset + " " + dim + "·" + reset + " " +
			dimGray + "/clear" + reset + " " + dim + "reset session" + reset
	}
	if pctUsed >= 40 {
		return dimGray + "/compact" + reset + " " + dim + "free context" + reset + " " + dim + "·" + reset + " " +
			dimGray + "shift+tab" + reset + " " + dim + "interrupt" + reset + " " + dim + "·" + reset + " " +
			dimGray + "esc esc" + reset + " " + dim + "cancel" + reset
	}
	switch time.Now().Unix() % 4 {
	case 0:
		return dimGray + "shift+tab" + reset + " " + dim + "interrupt" + reset + " " + dim + "·" + reset + " " +
			dimGray + "/compact" + reset + " " + dim + "free context" + reset + " " + dim + "·" + reset + " " +
			dimGray + "esc esc" + reset + " " + dim + "cancel" + reset
	case 1:
		return dimGray + "ctrl+r" + reset + " " + dim + "retry" + reset + " " + dim + "·" + reset + " " +
			dimGray + "#" + reset + " " + dim + "add files" + reset + " " + dim + "·" + reset + " " +
			dimGray + "/cost" + reset + " " + dim + "session cost" + reset
	case 2:
		return dimGray + "/init" + reset + " " + dim + "setup CLAUDE.md" + reset + " " + dim + "·" + reset + " " +
			dimGray + "/review" + reset + " " + dim + "review changes" + reset + " " + dim + "·" + reset + " " +
			dimGray + "/help" + reset + " " + dim + "commands" + reset
	default:
		return dimGray + "/model" + reset + " " + dim + "switch model" + reset + " " + dim + "·" + reset + " " +
			dimGray + "/vim" + reset + " " + dim + "vim mode" + reset + " " + dim + "·" + reset + " " +
			dimGray + "/config" + reset + " " + dim + "settings" + reset
	}
}
