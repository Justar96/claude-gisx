package app

import (
	"encoding/json"
	"fmt"
	"os"
)

// The manual-JSON block and the "how it runs" walkthrough used to live here.
// Both are gone: setup writes that JSON for you, and anyone who wants the
// long version has the repo. What's left is the two things you'd open this
// screen to look up.
func helpScreen() {
	banner()
	section("COMMANDS", []helpItem{
		{"setup", "wire into ~/.claude/settings.json (backs up any existing statusLine)"},
		{"status", "show install state and backup"},
		{"update", "download the latest release (--check to only look)"},
		{"uninstall", "restore your previous statusLine"},
		{"help", "this screen"},
	})
	section("ENV", []helpItem{
		{"CLAUDE_AUTOCOMPACT_PCT_OVERRIDE", "Claude Code's compact %; statusline shows a red ⚠ when reached"},
		{"CLAUDE_CODE_AUTO_COMPACT_WINDOW", "shrink the effective compaction window"},
		{"DISABLE_COMPACT", "disables auto-compact; statusline shows a dim compact:off badge"},
		{"CLAUDE_GISX_PLUGIN", "shell command whose stdout replaces line 3"},
		{"CLAUDE_GISX_NO_UPDATE_CHECK", "don't check GitHub for new releases"},
	})
	fmt.Printf("  %sgithub.com/Justar96/claude-gisx%s\n\n", cyan, reset)
}

type helpItem struct{ name, desc string }

// Column width comes from the entries themselves — the old version padded the
// labels by hand inside the strings, which drifted every time one was edited.
func section(title string, items []helpItem) {
	fmt.Printf("  %s%s%s\n", bold, title, reset)
	w := 0
	for _, it := range items {
		if len(it.name) > w {
			w = len(it.name)
		}
	}
	for _, it := range items {
		fmt.Printf("    %s%-*s%s  %s%s%s\n", white, w, it.name, reset, dim, it.desc, reset)
	}
	fmt.Println()
}

func detectInstallState() (state, otherCmd string) {
	raw, err := os.ReadFile(settingsPath())
	if err != nil {
		return "none", ""
	}
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		return "none", ""
	}
	sl, ok := m["statusLine"].(map[string]any)
	if !ok {
		return "none", ""
	}
	cmd, _ := sl["command"].(string)
	if cmd == "" {
		return "none", ""
	}
	if oursRe.MatchString(cmd) {
		return "active", ""
	}
	return "other", cmd
}
