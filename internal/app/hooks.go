package app

import (
	"os"
	"regexp"
	"runtime"
	"strings"
)

// Earlier releases could install a `claude-gisx hook prompt` UserPromptSubmit
// hook. It's gone; setup, update and uninstall strip the entry so it doesn't
// fail on every prompt, and `hook prompt` itself exits 0 meanwhile.

var ourHookRe = regexp.MustCompile(`\bclaude-gisx\b.*\bhook\s+prompt\b`)

// selfCommand is how settings.json should invoke this binary. See installCmd
// for why Windows gets an absolute forward-slash path.
func selfCommand() string {
	if runtime.GOOS == "windows" {
		if exe, err := os.Executable(); err == nil {
			return strings.ReplaceAll(exe, `\`, "/")
		}
	}
	return "claude-gisx"
}

func isOurHookEntry(entry any) bool {
	m, _ := entry.(map[string]any)
	inner, _ := m["hooks"].([]any)
	for _, h := range inner {
		hm, _ := h.(map[string]any)
		if cmd, _ := hm["command"].(string); ourHookRe.MatchString(cmd) {
			return true
		}
	}
	return false
}

// removePromptHook drops our entry and any container it leaves empty, so the
// cleanup doesn't leave a `"hooks": {}` behind that the user never wrote.
func removePromptHook(s map[string]any) bool {
	hooks, _ := s["hooks"].(map[string]any)
	list, _ := hooks["UserPromptSubmit"].([]any)
	var kept []any
	removed := false
	for _, e := range list {
		if isOurHookEntry(e) {
			removed = true
			continue
		}
		kept = append(kept, e)
	}
	if !removed {
		return false
	}
	if len(kept) == 0 {
		delete(hooks, "UserPromptSubmit")
	} else {
		hooks["UserPromptSubmit"] = kept
	}
	if len(hooks) == 0 {
		delete(s, "hooks")
	}
	return true
}

// cleanupLegacy removes the old prompt hook from settings.json. Older
// binaries run `hook install` on the new one after an update, which lands
// here too.
func cleanupLegacy() int {
	s := readSettings()
	if !removePromptHook(s) {
		return 0
	}
	if err := writeJSONFile(settingsPath(), s); err != nil {
		step(failMark, "prompt hook", dimGray+err.Error()+reset)
		return 1
	}
	step(okMark, "prompt hook", dimGray+"removed (no longer part of claude-gisx)"+reset)
	return 0
}
