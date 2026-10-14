package app

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// compactState is where Claude Code will auto-compact this session, worked
// out the way Claude Code does it (v2.1.299):
//
//	window    = CLAUDE_CODE_AUTO_COMPACT_WINDOW, else the settings'
//	            autoCompactWindow (per model, then top level); at least
//	            100k and never more than the model's context window
//	threshold = window − 13k, or window × CLAUDE_AUTOCOMPACT_PCT_OVERRIDE%
//	            when that is lower
//
// When none of those are set, Claude Code compacts at a window it tunes per
// model and doesn't expose, so threshold stays 0 and the bar shows the raw
// context window instead of guessing.
type compactState struct {
	threshold int  // tokens at which auto-compact fires; 0 when not configured
	off       bool // DISABLE_COMPACT or DISABLE_AUTO_COMPACT is in effect
}

const (
	compactBuffer    = 13_000  // Claude Code compacts this far short of the window
	minCompactWindow = 100_000 // smallest window Claude Code accepts
)

func computeCompactState(ctxSize int, modelID, projectDir string) compactState {
	for _, k := range []string{"DISABLE_COMPACT", "DISABLE_AUTO_COMPACT"} {
		if v := os.Getenv(k); v != "" && !isDisableFalsy(v) {
			return compactState{off: true}
		}
	}
	window := 0
	if v, err := strconv.Atoi(strings.TrimSpace(os.Getenv("CLAUDE_CODE_AUTO_COMPACT_WINDOW"))); err == nil && v > 0 {
		window = v
	} else {
		window = settingsCompactWindow(modelID, projectDir)
	}
	pct, _ := strconv.ParseFloat(strings.TrimSpace(os.Getenv("CLAUDE_AUTOCOMPACT_PCT_OVERRIDE")), 64)
	if pct <= 0 || pct > 100 {
		pct = 0
	}
	if window == 0 && pct == 0 {
		return compactState{}
	}
	if window == 0 {
		window = ctxSize
	} else {
		window = clampi(window, minCompactWindow, ctxSize)
	}
	threshold := window - compactBuffer
	if pct > 0 {
		threshold = mini(int(math.Floor(float64(window)*pct/100)), threshold)
	}
	if threshold <= 0 {
		return compactState{}
	}
	return compactState{threshold: threshold}
}

// settingsCompactWindow reads autoCompactWindow the way Claude Code merges
// it: user, project, then local settings, each later file winning. Within a
// file, modelSettings.<model>.autoCompactWindow beats the top-level value,
// and a file that sets the top-level value replaces earlier per-model ones.
// "auto" means Claude Code's own tuned window, which isn't knowable here.
func settingsCompactWindow(modelID, projectDir string) int {
	files := []string{settingsPath()}
	if projectDir != "" {
		files = append(files,
			filepath.Join(projectDir, ".claude", "settings.json"),
			filepath.Join(projectDir, ".claude", "settings.local.json"))
	}
	model := canonicalModel(modelID)
	var def, byModel any
	seen := map[string]bool{}
	for _, f := range files {
		// Opening Claude Code in ~ makes the project file the user file.
		if abs, err := filepath.Abs(f); err == nil {
			if seen[abs] {
				continue
			}
			seen[abs] = true
		}
		raw, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var s struct {
			AutoCompactWindow any `json:"autoCompactWindow"`
			ModelSettings     map[string]struct {
				AutoCompactWindow any `json:"autoCompactWindow"`
			} `json:"modelSettings"`
		}
		if json.Unmarshal(raw, &s) != nil {
			continue
		}
		var mine any
		for k, ms := range s.ModelSettings {
			if ms.AutoCompactWindow != nil && model != "" && canonicalModel(k) == model {
				mine = ms.AutoCompactWindow
			}
		}
		if s.AutoCompactWindow != nil {
			def, byModel = s.AutoCompactWindow, mine
		} else if mine != nil {
			byModel = mine
		}
	}
	v := def
	if byModel != nil {
		v = byModel
	}
	if n, ok := v.(float64); ok && n > 0 {
		return int(n)
	}
	return 0
}

var (
	modelVariantRe = regexp.MustCompile(`\[[^\]]*\]$`)
	modelDateRe    = regexp.MustCompile(`-\d{8}$`)
)

// canonicalModel maps the spellings Claude Code treats as one model —
// "claude-opus-5-5[1m]", a dated id, a Vertex "@date" id — onto one key.
func canonicalModel(id string) string {
	id = strings.ToLower(strings.TrimSpace(id))
	id = modelVariantRe.ReplaceAllString(id, "")
	if i := strings.IndexByte(id, '@'); i >= 0 {
		id = id[:i]
	}
	return modelDateRe.ReplaceAllString(id, "")
}

func isDisableFalsy(s string) bool {
	switch strings.ToLower(s) {
	case "", "0", "false":
		return true
	}
	return false
}
