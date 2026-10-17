package app

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Idle compaction (Claude Code v2.1.290+): a terminal conversation left idle
// is compacted shortly before its one-hour cache lapses, so the next message
// reads a summary instead of rewriting the whole prompt. Worked out the way
// Claude Code does it (v2.1.302):
//
//	mode      = the tengu_sunny_locket rollout flag Claude Code caches in
//	            ~/.claude.json; only "compact" and "compact_on_return" fire,
//	            and "idleCompaction": true does not turn on what it left off
//	minTokens = CLAUDE_CODE_IDLE_COMPACT_MIN_TOKENS, else the flag's
//	            minTokens; never below 100k, 200k when neither is set
//	fires at  = the last request + fireAtFraction (0.9, clamped to
//	            0.5–0.95) of the cache lifetime: about 54 minutes in
//
// and only while auto-compact is on, "idleCompaction" isn't false, and the
// cache is a warm one-hour entry holding at least minTokens.
const (
	idleFlagKey     = "tengu_sunny_locket"
	idleMinFloor    = 100_000
	idleMinDefault  = 200_000
	idleFraction    = 0.9
	idleFractionMin = 0.5
	idleFractionMax = 0.95
)

type idleFlag struct {
	Mode           string   `json:"mode"`
	MinTokens      *float64 `json:"minTokens"`
	FireAtFraction *float64 `json:"fireAtFraction"`
}

// idleCompactAt is when Claude Code compacts this conversation if nobody
// types, in unix seconds; 0 when it won't. prompt is the size of the cached
// prefix. The cheap payload checks go first, so a 5-minute session or a small
// prompt never opens ~/.claude.json.
func idleCompactAt(pc *promptCache, prompt int64, cs compactState, projectDir string) int64 {
	if pc == nil || !pc.Warm || pc.TTL != "1h" || pc.ExpiresAt <= 0 || cs.off || prompt < idleMinFloor {
		return 0
	}
	flag, autoCompact := readGlobalConfig()
	if flag == nil || (flag.Mode != "compact" && flag.Mode != "compact_on_return") || !autoCompact {
		return 0
	}
	if idleCompactionSetting(projectDir) == "false" {
		return 0
	}
	minTokens := int64(idleMinDefault)
	if v, err := strconv.ParseFloat(strings.TrimSpace(os.Getenv("CLAUDE_CODE_IDLE_COMPACT_MIN_TOKENS")), 64); err == nil && !math.IsInf(v, 0) {
		minTokens = max(idleMinFloor, int64(v))
	} else if flag.MinTokens != nil {
		minTokens = max(idleMinFloor, int64(*flag.MinTokens))
	}
	if prompt < minTokens {
		return 0
	}
	frac := idleFraction
	if flag.FireAtFraction != nil {
		frac = math.Min(idleFractionMax, math.Max(idleFractionMin, *flag.FireAtFraction))
	}
	// The entry was refreshed one TTL before it expires.
	return pc.ExpiresAt - int64(math.Round(3600*(1-frac)))
}

// globalConfigPath is Claude Code's own state file, which sits beside the
// config dir rather than in it unless CLAUDE_CONFIG_DIR moves both.
func globalConfigPath() string {
	if d := os.Getenv("CLAUDE_CONFIG_DIR"); d != "" {
		return filepath.Join(d, ".claude.json")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude.json")
}

// readGlobalConfig pulls the two keys idle compaction depends on out of
// ~/.claude.json: the cached rollout flag, and autoCompactEnabled (which
// /config writes there, not to settings.json).
func readGlobalConfig() (*idleFlag, bool) {
	raw, err := os.ReadFile(globalConfigPath())
	if err != nil {
		return nil, true
	}
	var c struct {
		Features struct {
			Flag *idleFlag `json:"tengu_sunny_locket"`
		} `json:"cachedGrowthBookFeatures"`
		AutoCompact *bool `json:"autoCompactEnabled"`
	}
	if json.Unmarshal(raw, &c) != nil {
		return nil, true
	}
	return c.Features.Flag, c.AutoCompact == nil || *c.AutoCompact
}

// idleCompactionSetting is "idleCompaction" as Claude Code merges it — user,
// project, then local settings, the later file winning — or "" when unset.
func idleCompactionSetting(projectDir string) string {
	files := []string{settingsPath()}
	if projectDir != "" {
		files = append(files,
			filepath.Join(projectDir, ".claude", "settings.json"),
			filepath.Join(projectDir, ".claude", "settings.local.json"))
	}
	v := ""
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var s struct {
			IdleCompaction *bool `json:"idleCompaction"`
		}
		if json.Unmarshal(raw, &s) == nil && s.IdleCompaction != nil {
			v = strconv.FormatBool(*s.IdleCompaction)
		}
	}
	return v
}
