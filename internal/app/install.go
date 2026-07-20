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

func banner() {
	fmt.Println()
	fmt.Printf("  %s%sclaude-gisx%s\n", bold, blue, reset)
	fmt.Printf("  %srich statusline for Claude Code%s\n", dimGray, reset)
	fmt.Println()
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
	switch {
	case len(s) == 0:
		fmt.Printf("  config   %smissing%s\n", dim, reset)
	case isOurs(s):
		sl, _ := s["statusLine"].(map[string]any)
		cmd, _ := sl["command"].(string)
		fmt.Printf("  config   %s active  %s%s%s\n", okMark, dim, cmd, reset)
	default:
		if sl, ok := s["statusLine"]; ok {
			fmt.Printf("  config   %sother%s  %s%s%s\n", yellow, reset, dim, jsonStr(sl), reset)
		} else {
			fmt.Printf("  config   %snone%s\n", dim, reset)
		}
	}
	switch {
	case prev.Found && prev.Had:
		fmt.Printf("  backup   %s %s%s%s\n", okMark, dim, jsonStr(prev.Value), reset)
	case prev.Found:
		fmt.Printf("  backup   %s(default)%s\n", dim, reset)
	default:
		fmt.Printf("  backup   %snone%s\n", dim, reset)
	}
	if _, err := os.Stat(backupFullPath()); err == nil {
		fmt.Printf("  snapshot %s%s%s\n", dim, backupFullPath(), reset)
	}
	fmt.Println()
	return 0
}
