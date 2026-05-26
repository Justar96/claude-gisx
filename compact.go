package main

import (
	"os"
	"strconv"
	"strings"
)

// compactState mirrors Claude Code's auto-compact behavior. pct is the
// threshold remapped onto the displayed used_percentage scale so a
// reduced CLAUDE_CODE_AUTO_COMPACT_WINDOW still aligns with the bar.
type compactState struct {
	pct     int  // threshold on used_percentage scale; 0 means "no badge"
	userSet bool // user configured at least one of the relevant env vars
	off     bool // DISABLE_COMPACT is in effect
}

func computeCompactState(ctxSize int) compactState {
	if v := os.Getenv("DISABLE_COMPACT"); v != "" && !isDisableFalsy(v) {
		return compactState{off: true}
	}
	if v := os.Getenv("CLAUDE_GISX_COMPACT_PCT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil && p > 0 {
			return compactState{pct: p, userSet: true}
		}
	}
	pctOverride := os.Getenv("CLAUDE_AUTOCOMPACT_PCT_OVERRIDE")
	winOverride := os.Getenv("CLAUDE_CODE_AUTO_COMPACT_WINDOW")
	if pctOverride == "" && winOverride == "" {
		return compactState{}
	}
	p := 95
	if pctOverride != "" {
		if v, err := strconv.Atoi(pctOverride); err == nil {
			p = v
		}
	}
	w := 0
	if winOverride != "" {
		if v, err := strconv.Atoi(winOverride); err == nil {
			w = v
		}
	}
	if w > 0 && ctxSize > 0 && w < ctxSize {
		return compactState{pct: (p * w) / ctxSize, userSet: true}
	}
	return compactState{pct: p, userSet: true}
}

func isDisableFalsy(s string) bool {
	switch strings.ToLower(s) {
	case "", "0", "false":
		return true
	}
	return false
}
