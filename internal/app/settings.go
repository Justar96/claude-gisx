package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"time"
)

func claudeDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude")
}

func settingsPath() string   { return filepath.Join(claudeDir(), "settings.json") }
func backupDir() string      { return filepath.Join(claudeDir(), ".gisx") }
func backupPrevPath() string { return filepath.Join(backupDir(), "prev-statusline.json") }
func backupFullPath() string { return filepath.Join(backupDir(), "settings.backup.json") }

func readJSONFile(p string) (map[string]any, error) {
	raw, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if len(raw) == 0 {
		return nil, nil
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		fmt.Fprintf(os.Stderr, "  %s invalid JSON: %s\n", failMark, p)
		return nil, nil
	}
	return m, nil
}

func writeJSONFile(p string, v any) error {
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	tmp := p + "." + strconv.Itoa(os.Getpid()) + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

// "Ours" detection — the command path can vary (PATH lookup, absolute path,
// or running from the compiled binary), so accept anything that ends in the
// claude-gisx binary name.
var oursRe = regexp.MustCompile(`\bclaude-gisx\b`)

func isOurs(settings map[string]any) bool {
	sl, ok := settings["statusLine"].(map[string]any)
	if !ok {
		return false
	}
	cmd, _ := sl["command"].(string)
	return cmd != "" && oursRe.MatchString(cmd)
}

func saveBackup(settings map[string]any) error {
	_, had := settings["statusLine"]
	full := map[string]any{
		"ts":       time.Now().UTC().Format(time.RFC3339),
		"settings": settings,
	}
	prev := map[string]any{
		"ts":  time.Now().UTC().Format(time.RFC3339),
		"had": had,
	}
	if had {
		prev["value"] = settings["statusLine"]
	} else {
		prev["value"] = nil
	}
	if err := writeJSONFile(backupFullPath(), full); err != nil {
		return err
	}
	return writeJSONFile(backupPrevPath(), prev)
}

type prevState struct {
	Found bool
	Had   bool
	Value any
}

func loadPrev() prevState {
	d, err := readJSONFile(backupPrevPath())
	if err != nil || d == nil {
		return prevState{}
	}
	had, _ := d["had"].(bool)
	return prevState{Found: true, Had: had, Value: d["value"]}
}

func cleanBackup() {
	_ = os.Remove(backupPrevPath())
	_ = os.Remove(backupFullPath())
	_ = os.Remove(backupDir())
}

func readSettings() map[string]any {
	m, _ := readJSONFile(settingsPath())
	if m == nil {
		return map[string]any{}
	}
	return m
}
