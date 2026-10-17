package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// ModFS is mods/gisx, embedded by the root package: the companion mod that
// hands the status line Claude Code's /context breakdown (see ctxmap.go).
// Claude Code 2.1.287+ auto-loads a plugin folder under ~/.claude/skills/ as
// gisx@skills-dir, so installing it is copying the folder there.
var ModFS fs.FS

const modMarker = "claude-gisx" // plugin.json metadata.installedBy

func modDir() string { return filepath.Join(claudeDir(), "skills", "gisx") }

// modFiles is the folder as it should stand on disk: the embedded files less
// the tests, with env.ts pointing at this binary so the mod never depends on
// the PATH Claude Code was started with.
func modFiles() (map[string][]byte, error) {
	if ModFS == nil {
		return nil, errors.New("this build carries no mod")
	}
	files := map[string][]byte{}
	err := fs.WalkDir(ModFS, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || strings.HasPrefix(p, "tests/") {
			return err
		}
		b, err := fs.ReadFile(ModFS, p)
		files[p] = b
		return err
	})
	if err != nil {
		return nil, err
	}
	files["hooks/env.ts"] = []byte("// Written by `claude-gisx setup`: the binary the mod pipes the breakdown to.\n" +
		"export const GISX = " + strconv.Quote(modBinary()) + "\n")
	return files, nil
}

func modBinary() string {
	exe, err := os.Executable()
	if err != nil {
		return "claude-gisx"
	}
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real
	}
	if runtime.GOOS == "windows" {
		return strings.ReplaceAll(exe, `\`, "/")
	}
	return exe
}

// isOurMod says whether the folder at dir is one setup wrote, so neither
// install nor uninstall ever touches a gisx plugin someone else put there.
func isOurMod(dir string) bool {
	raw, err := os.ReadFile(filepath.Join(dir, ".claude-plugin", "plugin.json"))
	if err != nil {
		return false
	}
	var m struct {
		Metadata struct {
			InstalledBy string `json:"installedBy"`
		} `json:"metadata"`
	}
	return json.Unmarshal(raw, &m) == nil && m.Metadata.InstalledBy == modMarker
}

// installMod writes the mod into place and says what it did: "installed",
// "updated" or "up to date". Claude Code watches the folder and reloads a
// changed mod, so it's built beside the target and renamed in whole — a
// session never loads half of it — and an unchanged one isn't touched.
func installMod() (string, error) {
	files, err := modFiles()
	if err != nil {
		return "", err
	}
	dir := modDir()
	_, statErr := os.Stat(dir)
	exists := statErr == nil
	if exists {
		if !isOurMod(dir) {
			return "", fmt.Errorf("%s holds a gisx plugin claude-gisx didn't write", dir)
		}
		if sameFiles(dir, files) {
			return "up to date", nil
		}
	}
	tmp := filepath.Join(filepath.Dir(dir), ".gisx.tmp-"+strconv.Itoa(os.Getpid()))
	_ = os.RemoveAll(tmp)
	for p, b := range files {
		dst := filepath.Join(tmp, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return "", err
		}
		if err := os.WriteFile(dst, b, 0o644); err != nil {
			_ = os.RemoveAll(tmp)
			return "", err
		}
	}
	// A dot-folder: Claude Code skips hidden entries while it scans, so
	// neither the staging copy nor the outgoing one loads as a second gisx.
	old := filepath.Join(filepath.Dir(dir), ".gisx.old-"+strconv.Itoa(os.Getpid()))
	if exists {
		if err := os.Rename(dir, old); err != nil {
			_ = os.RemoveAll(tmp)
			return "", err
		}
	}
	if err := os.Rename(tmp, dir); err != nil {
		if exists {
			_ = os.Rename(old, dir)
		}
		_ = os.RemoveAll(tmp)
		return "", err
	}
	_ = os.RemoveAll(old)
	if exists {
		return "updated", nil
	}
	return "installed", nil
}

// sameFiles is true when dir holds exactly files: Claude Code writes its
// type declarations into .claude-plugin/types/ on load, which doesn't count.
func sameFiles(dir string, files map[string][]byte) bool {
	seen := 0
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if rel == ".claude-plugin/types" {
				return fs.SkipDir
			}
			return nil
		}
		want, ok := files[rel]
		if !ok {
			return errors.New("extra")
		}
		got, err := os.ReadFile(p)
		if err != nil || !bytes.Equal(got, want) {
			return errors.New("differs")
		}
		seen++
		return nil
	})
	return err == nil && seen == len(files)
}

// removeMod deletes the folder setup wrote, and only that one.
func removeMod() (bool, error) {
	dir := modDir()
	if !isOurMod(dir) {
		return false, nil
	}
	return true, os.RemoveAll(dir)
}

// modVersion is the installed mod's manifest version, or "" without one.
func modVersion() string {
	raw, err := os.ReadFile(filepath.Join(modDir(), ".claude-plugin", "plugin.json"))
	if err != nil {
		return ""
	}
	var m struct {
		Version string `json:"version"`
	}
	_ = json.Unmarshal(raw, &m)
	return m.Version
}
