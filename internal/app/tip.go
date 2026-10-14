package app

import "time"

// tipContext is everything the tip line (line 4) needs to pick what to say.
type tipContext struct {
	plugin string
	update string // newer release tag, if the cached check found one
}

func tipLine(t tipContext) string {
	if t.plugin != "" {
		return t.plugin
	}
	// Rare and actionable, so it outranks the rotation — but not a user's own
	// plugin line.
	if tag := t.update; tag != "" {
		return pinkTint("✦ claude-gisx "+tag+" available") + " " + dim + "·" + reset + " " +
			dimGray + "claude-gisx update" + reset
	}

	pool := whatsNew()
	if len(pool) == 0 {
		return ""
	}
	return pool[int(time.Now().Unix())%len(pool)]
}
