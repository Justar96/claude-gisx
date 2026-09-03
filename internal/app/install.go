package app

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
)

func jsonStr(v any) string {
	raw, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(raw)
}

var (
	okMark   = green + "✓" + reset
	failMark = red + "✗" + reset
	dotMark  = dimGray + "·" + reset
)

type installOpts struct {
	force   bool
	noCheck bool
}

// The mark is drawn from the same heavy box-drawing family as the statusline's
// context bar, so the installer and the thing it installs read as one tool.
// Version, tagline and install state sit alongside it rather than below, which
// keeps every screen's header to three lines.
func banner() {
	mark := [3]string{
		"┏━╸╻┏━┓╻ ╻",
		"┃╺┓┃┗━┓┏╋┛",
		"┗━┛╹┗━┛╹ ╹",
	}
	state, _ := detectInstallState()
	meta := [3]string{
		bold + white + "claude-gisx" + reset + " " + dim + verLabel(Version) + reset,
		dimGray + "statusline for Claude Code" + reset,
		stateBadge(state),
	}
	fmt.Println()
	for i := range mark {
		fmt.Printf("  %s%s%s   %s\n", bold+blue, mark[i], reset, meta[i])
	}
	fmt.Println()
}

// Release tags carry a "v" but the linker-injected version sometimes doesn't,
// so every version on screen goes through here rather than being printed raw —
// otherwise a line reads "1.3.0 → v1.3.1".
func verLabel(v string) string { return "v" + strings.TrimPrefix(v, "v") }

// The offending command is left out here — status and setup both print it in
// full, and the banner only needs to say which of the three states you're in.
func stateBadge(state string) string {
	switch state {
	case "active":
		return green + "●" + reset + " active"
	case "other":
		return yellow + "●" + reset + " another statusLine is active"
	default:
		return dimGray + "○" + reset + " not installed"
	}
}

func preview() {
	fmt.Printf("  %spreview%s\n", dimGray, reset)
	fmt.Printf("  %s%sClaude Opus 4.8%s%s/%s%shigh%s %s %s██%s██%s%s░░░░░░░░░░░%s %s28%%%s%s/%s%s1M%s %s+ext%s %s %sclaude-gisx%s%s:%s%smain%s %s %s12m%s %s %s$1.20%s\n",
		bold, blue, reset, dim, reset, cyan, reset, dotMark,
		green, cyan, reset, dimGray, reset,
		green, reset, dim, reset, dimGray, reset,
		dimGray, reset,
		dotMark, cyan, reset, dim, reset, green, reset,
		dotMark, white, reset,
		dotMark, white, reset,
	)
	fmt.Printf("  %s5h%s %s12%%%s %sresets%s %s4h 2m%s %s %s7d%s %s3%%%s %sresets%s %s6d 1h%s %s %sFable%s %s61%%%s %sresets%s %s6d 1h%s %s %s15.7M%s %s▲18%%%s\n",
		white, reset, green, reset, dimGray, reset, green, reset,
		dotMark, white, reset, green, reset, dimGray, reset, green, reset,
		dotMark, white, reset, orange, reset, dimGray, reset, yellow, reset,
		dotMark, white, reset, green, reset,
	)
	fmt.Printf("  %snew in 2.1.215%s %s %sClaude no longer runs /verify and /code-review on its own%s\n",
		dimGray, reset, dotMark, dim, reset,
	)
	fmt.Println()
}

func bin(cmd string) string {
	lookup := "command"
	if runtime.GOOS == "windows" {
		lookup = "where"
	}
	out, err := exec.Command(lookup, "-v", cmd).Output()
	if err != nil {
		// fallback: try `which` / `where`
		if runtime.GOOS == "windows" {
			out, err = exec.Command("where", cmd).Output()
		} else {
			out, err = exec.Command("which", cmd).Output()
		}
		if err != nil {
			return ""
		}
	}
	return strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
}

var verRe = regexp.MustCompile(`(\d+\.\d+(?:\.\d+)*)`)

func ver(cmd string) string {
	out, err := exec.Command(cmd, "--version").CombinedOutput()
	if err != nil {
		return ""
	}
	m := verRe.FindString(string(out))
	return m
}

func checkDeps() bool {
	deps := []struct {
		name     string
		required bool
	}{
		{"git", false},
		{"curl", false},
	}
	pass := true
	for _, d := range deps {
		path := bin(d.name)
		if path != "" {
			v := ver(d.name)
			if v != "" {
				fmt.Printf("  %s %s %s%s%s\n", okMark, d.name, dimGray, v, reset)
			} else {
				fmt.Printf("  %s %s\n", okMark, d.name)
			}
		} else if d.required {
			fmt.Printf("  %s %s %snot found%s\n", failMark, d.name, red, reset)
			pass = false
		} else {
			fmt.Printf("  %s- %s (optional)%s\n", dim, d.name, reset)
		}
	}
	return pass
}

func installCmd(opts installOpts) int {
	banner()
	s := readSettings()

	if isOurs(s) && !opts.force {
		fmt.Printf("  %s already active\n", okMark)
		fmt.Printf("  %s--force to reinstall, status to inspect%s\n\n", dim, reset)
		return 0
	}

	if !opts.noCheck {
		fmt.Printf("  %sdependencies%s\n", dimGray, reset)
		if !checkDeps() {
			fmt.Println()
			fmt.Printf("  %s missing required dependencies\n", failMark)
			fmt.Printf("  %sinstall them and retry, or --no-check to skip%s\n\n", dim, reset)
			return 1
		}
		fmt.Println()
	}

	preview()

	// On --force reinstall the existing entry is already ours; overwriting the
	// backup would clobber the original pre-gisx statusLine.
	if !isOurs(s) {
		if err := saveBackup(s); err != nil {
			fmt.Fprintf(os.Stderr, "  %s backup failed: %v\n", failMark, err)
			return 1
		}
		if sl, ok := s["statusLine"]; ok {
			fmt.Printf("  %s backed up: %s%s%s\n", dotMark, dim, jsonStr(sl), reset)
		}
	}

	// On Windows, Claude Code routes statusLine.command through Git Bash (when
	// present) or PowerShell. Git Bash eats backslashes as escape characters,
	// so an absolute path written with native separators silently breaks —
	// the binary launches but stdout disappears. Forward slashes work in both
	// shells and in Windows native CreateProcess. Linux/macOS keep the bare
	// name so PATH does the work.
	command := "claude-gisx"
	if runtime.GOOS == "windows" {
		if exe, err := os.Executable(); err == nil {
			command = strings.ReplaceAll(exe, `\`, "/")
		}
	}
	// Preserve any extra keys (e.g. statusLine.padding) the user added to our
	// entry. We only overwrite the two fields we actually own.
	var sl map[string]any
	if isOurs(s) {
		sl, _ = s["statusLine"].(map[string]any)
	}
	if sl == nil {
		sl = map[string]any{}
	}
	sl["type"] = "command"
	sl["command"] = command
	// We render vim.mode ourselves; without this Claude Code also prints
	// "-- INSERT --" under the prompt. Left alone if the user set it.
	if _, ok := sl["hideVimModeIndicator"]; !ok {
		sl["hideVimModeIndicator"] = true
	}
	s["statusLine"] = sl
	if err := writeJSONFile(settingsPath(), s); err != nil {
		fmt.Fprintf(os.Stderr, "  %s write failed: %v\n", failMark, err)
		return 1
	}
	fmt.Printf("  %s installed\n", okMark)
	fmt.Printf("\n  %srestart Claude Code to apply%s\n\n", dim, reset)
	return 0
}

func uninstallCmd(opts installOpts) int {
	banner()
	s := readSettings()
	if len(s) == 0 {
		fmt.Printf("  %snothing to do%s\n", dim, reset)
		cleanBackup()
		return 0
	}
	if sl, ok := s["statusLine"]; ok && !isOurs(s) && !opts.force {
		fmt.Printf("  %s statusLine belongs to: %s\n", failMark, jsonStr(sl))
		fmt.Printf("  %spass --force to override%s\n", dim, reset)
		return 1
	}
	prev := loadPrev()
	if prev.Found && prev.Had {
		s["statusLine"] = prev.Value
		fmt.Printf("  %s restored: %s%s%s\n", okMark, dim, jsonStr(prev.Value), reset)
	} else {
		delete(s, "statusLine")
		fmt.Printf("  %s restored: %sdefault%s\n", okMark, dim, reset)
	}
	if err := writeJSONFile(settingsPath(), s); err != nil {
		fmt.Fprintf(os.Stderr, "  %s write failed: %v\n", failMark, err)
		return 1
	}
	cleanBackup()
	fmt.Printf("  %s cleaned up backups\n", okMark)
	fmt.Printf("\n  %srestart Claude Code to apply%s\n\n", dim, reset)
	return 0
}

func statusCmd() int {
	s := readSettings()
	prev := loadPrev()
	banner()
	row := func(label, value string) {
		fmt.Printf("  %s%-8s%s  %s\n", dimGray, label, reset, value)
	}
	switch {
	case len(s) == 0:
		row("config", dim+"missing"+reset)
	case isOurs(s):
		sl, _ := s["statusLine"].(map[string]any)
		cmd, _ := sl["command"].(string)
		row("config", okMark+" active  "+dim+cmd+reset)
	default:
		if sl, ok := s["statusLine"]; ok {
			row("config", yellow+"other"+reset+"  "+dim+jsonStr(sl)+reset)
		} else {
			row("config", dim+"none"+reset)
		}
	}
	switch {
	case prev.Found && prev.Had:
		row("backup", okMark+" "+dim+jsonStr(prev.Value)+reset)
	case prev.Found:
		row("backup", dim+"(default)"+reset)
	default:
		row("backup", dim+"none"+reset)
	}
	if _, err := os.Stat(backupFullPath()); err == nil {
		row("snapshot", dim+backupFullPath()+reset)
	}
	fmt.Println()
	return 0
}
