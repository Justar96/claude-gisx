package main

import (
	"encoding/json"
	"fmt"
	"os"
)

func helpScreen() {
	state, otherCmd := detectInstallState()
	fmt.Println()
	fmt.Printf("  %s%sclaude-gisx%s %sv%s%s\n", bold, blue, reset, dim, version, reset)
	fmt.Printf("  %srich, dynamic statusline for Claude Code%s\n\n", dimGray, reset)

	switch {
	case state == "active":
		fmt.Printf("  %s%sinstalled%s %s— restart Claude Code if you just installed%s\n",
			green, "✓ ", reset, dim, reset)
	case state == "other":
		fmt.Printf("  %s!%s another statusLine is active: %s%s%s\n", yellow, reset, dim, otherCmd, reset)
		fmt.Printf("    %srun %s%sclaude-gisx setup%s %sto back it up and switch%s\n",
			dim, reset, cyan, reset, dim, reset)
	default:
		fmt.Printf("  %s○%s not installed yet\n", dimGray, reset)
	}
	fmt.Println()

	fmt.Printf("  %sGet started%s\n", bold, reset)
	bullet("claude-gisx setup     ", "wire into ~/.claude/settings.json (backs up any existing statusLine)")
	bullet("claude-gisx status    ", "show current install state and backup")
	bullet("claude-gisx uninstall ", "restore your previous statusLine")
	bullet("claude-gisx help      ", "this screen")
	fmt.Println()

	fmt.Printf("  %sOr wire it manually%s\n", bold, reset)
	fmt.Printf("  %sadd to ~/.claude/settings.json%s\n\n", dim, reset)
	fmt.Printf("    %s{%s\n", dim, reset)
	fmt.Printf("      %s\"statusLine\"%s: {\n", cyan, reset)
	fmt.Printf("        %s\"type\"%s: %s\"command\"%s,\n", cyan, reset, green, reset)
	fmt.Printf("        %s\"command\"%s: %s\"claude-gisx\"%s\n", cyan, reset, green, reset)
	fmt.Printf("      }\n")
	fmt.Printf("    %s}%s\n\n", dim, reset)

	fmt.Printf("  %sConfigure%s %s(all optional, all env-vars)%s\n", bold, reset, dim, reset)
	bullet("CLAUDE_AUTOCOMPACT_PCT_OVERRIDE", "Claude Code's compact %; statusline shows a red ⚠ when reached")
	bullet("CLAUDE_CODE_AUTO_COMPACT_WINDOW", "shrink the effective compaction window; badge remaps onto used %")
	bullet("DISABLE_COMPACT                ", "disables auto-compact; statusline shows a dim compact:off badge")
	bullet("CLAUDE_GISX_PLUGIN             ", "shell command whose stdout replaces the 3rd line (your own API, etc.)")
	fmt.Println()

	fmt.Printf("  %sHow it runs%s\n", bold, reset)
	fmt.Printf("  %sClaude Code pipes a JSON session blob to %s%sclaude-gisx%s%s on stdin.%s\n",
		dim, reset, cyan, reset, dim, reset)
	fmt.Printf("  %sYou can preview the output by piping JSON manually:%s\n\n", dim, reset)
	fmt.Printf("    %secho%s %s'{\"model\":{\"display_name\":\"Opus\"},\"context_window\":{\"used_percentage\":12}}'%s %s| %s%sclaude-gisx%s\n\n",
		cyan, reset, green, reset, dim, reset, cyan, reset)

	fmt.Printf("  %sdocs: %s%shttps://github.com/Justar96/claude-gisx%s\n\n", dim, reset, cyan, reset)
}

func bullet(label, body string) {
	fmt.Printf("  %s•%s %s%s%s  %s%s%s\n", dimGray, reset, white, label, reset, dim, body, reset)
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
