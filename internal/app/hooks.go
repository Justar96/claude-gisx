package app

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
)

// The prompt-rewrite hook is offered, never forced: it sends prompts to
// third-party APIs, so setup asks once, remembers a "no", and only then stops
// asking. Accepting writes one UserPromptSubmit entry into settings.json next
// to whatever hooks are already there.

// Set to "off" in any config layer to never be asked; a "no" at the prompt
// writes exactly that into ~/.claude/.gisx/config.json.
const promptHookOptOut = "CLAUDE_GISX_PROMPT_HOOK"

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

func promptHookEntries(s map[string]any) []any {
	hooks, _ := s["hooks"].(map[string]any)
	list, _ := hooks["UserPromptSubmit"].([]any)
	return list
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

func hasPromptHook(s map[string]any) bool {
	for _, e := range promptHookEntries(s) {
		if isOurHookEntry(e) {
			return true
		}
	}
	return false
}

func addPromptHook(s map[string]any) {
	if hasPromptHook(s) {
		return
	}
	hooks, _ := s["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
	}
	entry := map[string]any{"hooks": []any{map[string]any{
		"type":    "command",
		"command": selfCommand() + " hook prompt",
		// Each API call is capped well below this; it's the backstop that
		// keeps a wedged process from holding the prompt for Claude Code's 30s.
		"timeout": 15,
	}}}
	hooks["UserPromptSubmit"] = append(promptHookEntries(s), entry)
	s["hooks"] = hooks
}

// removePromptHook drops our entry and any container it leaves empty, so
// uninstall doesn't leave a `"hooks": {}` behind that the user never wrote.
func removePromptHook(s map[string]any) bool {
	var kept []any
	removed := false
	for _, e := range promptHookEntries(s) {
		if isOurHookEntry(e) {
			removed = true
			continue
		}
		kept = append(kept, e)
	}
	if !removed {
		return false
	}
	hooks := s["hooks"].(map[string]any)
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

// hookChoice is how the caller wants the offer handled.
type hookChoice int

const (
	hookAsk hookChoice = iota // ask on a terminal, hint otherwise
	hookYes                   // --hook: install without asking
	hookNo                    // --no-hook: leave it alone this time
)

func hookChoiceFrom(flags map[string]bool) hookChoice {
	switch {
	case flags["hook"] || flags["yes"]:
		return hookYes
	case flags["no-hook"] || flags["no"]:
		return hookNo
	}
	return hookAsk
}

// offerPromptHook installs the hook into s when the user agrees and reports
// whether s changed. Everything it prints is one settled line.
func offerPromptHook(s map[string]any, choice hookChoice) bool {
	if hasPromptHook(s) {
		step(okMark, "prompt hook", dimGray+"installed"+reset)
		return false
	}
	if choice == hookNo {
		return false
	}
	if choice == hookAsk {
		if isOff(loadConfig("").value(promptHookOptOut)) {
			return false
		}
		answer, ok := askYesNo("  Install the prompt-rewrite hook? It sends unclear prompts to DeepSeek\n" +
			"  (and TypeSafe, to judge them) once DEEPSEEK_API_KEY is set. [y/N] ")
		if !ok {
			step(dotMark, "prompt hook", dimGray+"not installed · claude-gisx setup --hook"+reset)
			return false
		}
		if !answer {
			_ = setGisxConfig(promptHookOptOut, "off")
			step(dotMark, "prompt hook", dimGray+"skipped · claude-gisx setup --hook to add it later"+reset)
			return false
		}
	}
	// An explicit yes overrides an earlier no.
	_ = setGisxConfig(promptHookOptOut, "")
	addPromptHook(s)
	step(okMark, "prompt hook", dimGray+"installed · set DEEPSEEK_API_KEY to use it"+reset)
	return true
}

// askYesNo reads one answer from the terminal. ok is false when there's no
// terminal to ask on.
func askYesNo(question string) (yes, ok bool) {
	line, ok := ask(question, false)
	switch strings.ToLower(line) {
	case "y", "yes":
		return true, ok
	}
	return false, ok
}

// ask is askLine, swappable so tests never block on a real terminal.
var ask = askLine

// askLine reads one line from the controlling terminal rather than stdin:
// the installer runs as `curl | bash`, where stdin is the script itself.
// hidden turns echo off while typing, for secrets.
func askLine(question string, hidden bool) (string, bool) {
	inName, outName := "/dev/tty", "/dev/tty"
	if runtime.GOOS == "windows" {
		inName, outName = "CONIN$", "CONOUT$"
	}
	in, err := os.OpenFile(inName, os.O_RDWR, 0)
	if err != nil {
		return "", false
	}
	defer in.Close()
	out, err := os.OpenFile(outName, os.O_RDWR, 0)
	if err != nil {
		return "", false
	}
	defer out.Close()
	if !isCharDevice(in) {
		return "", false
	}
	fmt.Fprint(out, question)
	if hidden && runtime.GOOS != "windows" {
		if setEcho(in, false) == nil {
			defer func() { _ = setEcho(in, true); fmt.Fprintln(out) }()
		}
	}
	line, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && line == "" {
		return "", false
	}
	return strings.TrimSpace(line), true
}

// setEcho flips terminal echo with stty, which every Unix has; pulling in a
// terminal package for one flag isn't worth a dependency.
func setEcho(tty *os.File, on bool) error {
	arg := "-echo"
	if on {
		arg = "echo"
	}
	cmd := exec.Command("stty", arg)
	cmd.Stdin = tty
	return cmd.Run()
}

// apiKeys are the keys setup offers to store, in the order it asks.
var apiKeys = []struct {
	name, what, where string
}{
	{"TYPESAFE_API_KEY", "TypeSafe", "next-prompt suggestions, rewrite checks · console.typesafe.ai/keys"},
	{"DEEPSEEK_API_KEY", "DeepSeek", "prompt rewrites · platform.deepseek.com/api_keys"},
}

// offerAPIKeys asks for each key no config layer has yet and stores what the
// user pastes in ~/.claude/.gisx/config.json. DeepSeek is only asked for when
// the prompt hook is installed, since nothing else uses it. A key found only
// in a project file works only in that project, so setup offers to keep a
// copy for all of them. always (the `keys` command) asks for every key, with
// Enter keeping the current one. Keys are reported by source, never printed.
func offerAPIKeys(projectDir string, hookInstalled, always bool) {
	cfg := loadConfig(projectDir)
	for _, k := range apiKeys {
		if k.name == "DEEPSEEK_API_KEY" && !hookInstalled && !always {
			continue
		}
		val, src := cfg.get(k.name)
		if src != "" && !always {
			if !projectScoped(src) || globalValue(k.name) == val {
				step(okMark, k.what+" key", dimGray+"from "+sourceLabel(src)+reset)
				continue
			}
			yes, ok := askYesNoDefault(fmt.Sprintf("  %s key found in %s, which only applies there.\n"+
				"  Save it to %s for every project? [Y/n] ", k.what, src, gisxConfigPath()), true)
			if !ok || !yes {
				step(dotMark, k.what+" key", dimGray+"from "+src+" · this project only"+reset)
				continue
			}
			saveKey(k.what, k.name, val)
			continue
		}
		prompt := fmt.Sprintf("  %s API key for %s (Enter to skip): ", k.what, k.where)
		if src != "" {
			prompt = fmt.Sprintf("  %s API key, now from %s (Enter to keep): ", k.what, sourceLabel(src))
		}
		in, ok := ask(prompt, true)
		switch {
		case !ok:
			step(dotMark, k.what+" key", dimGray+"not set · run claude-gisx keys in a terminal"+reset)
		case in == "" && src != "":
			step(okMark, k.what+" key", dimGray+"kept · "+sourceLabel(src)+reset)
		case in == "":
			step(dotMark, k.what+" key", dimGray+"skipped"+reset)
		default:
			saveKey(k.what, k.name, in)
		}
	}
}

func saveKey(what, name, val string) {
	if err := setGisxConfig(name, val); err != nil {
		step(failMark, what+" key", dimGray+err.Error()+reset)
		return
	}
	step(okMark, what+" key", dimGray+"saved to "+gisxConfigPath()+reset)
}

// globalValue is a key's value in the layers that apply to every project,
// ignoring project files and the environment.
func globalValue(name string) string {
	for _, l := range loadConfig("") {
		if v := l.vars[name]; v != "" {
			return v
		}
	}
	return ""
}

// projectScoped reports whether a config source only applies inside one
// project. The environment and the user-level files apply everywhere.
func projectScoped(src string) bool {
	return src != "env" && src != settingsPath() && src != gisxConfigPath()
}

func sourceLabel(src string) string {
	if src == "env" {
		return "environment"
	}
	return src
}

func askYesNoDefault(question string, def bool) (yes, ok bool) {
	line, ok := ask(question, false)
	switch strings.ToLower(line) {
	case "y", "yes":
		return true, ok
	case "n", "no":
		return false, ok
	}
	return def, ok
}

// keysCmd is `claude-gisx keys`: set or replace the API keys at any time.
func keysCmd() int {
	banner()
	cwd, _ := os.Getwd()
	offerAPIKeys(cwd, hasPromptHook(readSettings()), true)
	fmt.Println()
	return 0
}

// setGisxConfig writes one key into ~/.claude/.gisx/config.json, keeping the
// rest. An empty value deletes the key. The file can hold API keys, so it's
// kept owner-only.
func setGisxConfig(name, value string) error {
	m, _ := readJSONFile(gisxConfigPath())
	if m == nil {
		if value == "" {
			return nil
		}
		m = map[string]any{}
	}
	if value == "" {
		if _, had := m[name]; !had {
			return nil
		}
		delete(m, name)
	} else {
		m[name] = value
	}
	raw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	if err := writeFileAtomic(gisxConfigPath(), append(raw, '\n')); err != nil {
		return err
	}
	return os.Chmod(gisxConfigPath(), 0o600)
}

// hookInstallCmd is `claude-gisx hook install`: the offer on its own, which
// update runs through the freshly installed binary so the new version is the
// one that asks.
func hookInstallCmd(choice hookChoice) int {
	s := readSettings()
	if offerPromptHook(s, choice) {
		if err := writeJSONFile(settingsPath(), s); err != nil {
			fmt.Fprintf(os.Stderr, "  %s write failed: %v\n", failMark, err)
			return 1
		}
	}
	cwd, _ := os.Getwd()
	offerAPIKeys(cwd, hasPromptHook(s), false)
	return 0
}

func hookUninstallCmd() int {
	s := readSettings()
	if !removePromptHook(s) {
		step(dotMark, "prompt hook", dimGray+"not installed"+reset)
		return 0
	}
	if err := writeJSONFile(settingsPath(), s); err != nil {
		fmt.Fprintf(os.Stderr, "  %s write failed: %v\n", failMark, err)
		return 1
	}
	step(okMark, "prompt hook", dimGray+"removed"+reset)
	return 0
}

// promptHookNudge is a rotation entry for anyone who got this version through
// an older `update`, which had no offer to make. It stops once the hook is in
// or declined.
func promptHookNudge() string {
	if isOff(loadConfig("").value(promptHookOptOut)) || hasPromptHook(readSettings()) {
		return ""
	}
	return dimGray + "new: prompt rewrite hook" + reset + " " + dim + "·" + reset + " " +
		dimGray + "claude-gisx hook install" + reset
}
