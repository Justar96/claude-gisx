package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Settings like TYPESAFE_API_KEY can live in a config file as well as the
// environment. Claude Code copies a settings file's `env` block into its own
// environment, but only for the files it loaded, and only after a restart. A
// key added mid-session, or set in a file the statusline's process never
// inherited, would otherwise go unseen. So the files are read here, highest
// precedence first:
//
//  1. the process environment
//  2. <project>/.claude/settings.local.json  `env` (gitignored; the place for secrets)
//  3. <project>/.env                         KEY=value lines
//  4. <project>/.claude/settings.json        `env`
//  5. ~/.claude/settings.json                `env`
//  6. ~/.claude/.gisx/config.json            flat {"NAME": "value"}

func gisxConfigPath() string { return filepath.Join(backupDir(), "config.json") }

type configLayer struct {
	path string
	vars map[string]string
}

// gisxConfig is loaded once per render, so each file is read at most once
// no matter how many settings are looked up.
type gisxConfig []configLayer

func loadConfig(projectDir string) gisxConfig {
	var paths []string
	if projectDir != "" {
		paths = append(paths,
			filepath.Join(projectDir, ".claude", "settings.local.json"),
			filepath.Join(projectDir, ".env"),
			filepath.Join(projectDir, ".claude", "settings.json"))
	}
	paths = append(paths, settingsPath())

	var cfg gisxConfig
	seen := map[string]bool{}
	for _, p := range paths {
		// Opening Claude Code in ~ makes the project files the user files.
		if abs, err := filepath.Abs(p); err == nil {
			if seen[abs] {
				continue
			}
			seen[abs] = true
		}
		read := settingsEnv
		if filepath.Base(p) == ".env" {
			read = dotenv
		}
		if vars := read(p); len(vars) > 0 {
			cfg = append(cfg, configLayer{p, vars})
		}
	}
	if vars := readVars(gisxConfigPath()); len(vars) > 0 {
		cfg = append(cfg, configLayer{gisxConfigPath(), vars})
	}
	return cfg
}

// get returns the value of name and where it came from ("env" for the
// process environment), or "" when no layer sets it.
func (c gisxConfig) get(name string) (value, source string) {
	if v := os.Getenv(name); v != "" {
		return v, "env"
	}
	for _, l := range c {
		if v := l.vars[name]; v != "" {
			return v, l.path
		}
	}
	return "", ""
}

func (c gisxConfig) value(name string) string {
	v, _ := c.get(name)
	return v
}

func settingsEnv(path string) map[string]string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var s struct {
		Env map[string]any `json:"env"`
	}
	if json.Unmarshal(raw, &s) != nil {
		return nil
	}
	return stringify(s.Env)
}

// dotenv reads KEY=value lines: blank lines and # comments are skipped, an
// `export ` prefix is allowed, and one pair of surrounding quotes is removed.
// No interpolation — a key is a literal string.
func dotenv(path string) map[string]string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	out := map[string]string{}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') && v[len(v)-1] == v[0] {
			v = v[1 : len(v)-1]
		} else if i := strings.Index(v, " #"); i >= 0 {
			v = strings.TrimSpace(v[:i])
		}
		if k != "" {
			out[k] = v
		}
	}
	return out
}

func readVars(path string) map[string]string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		return nil
	}
	return stringify(m)
}

// Settings files are hand-edited, so `"CLAUDE_GISX_SUGGEST_MIN": 0.4` is as
// likely as the string form.
func stringify(m map[string]any) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		switch v := v.(type) {
		case string:
			out[k] = v
		case float64, bool:
			out[k] = fmt.Sprint(v)
		}
	}
	return out
}
