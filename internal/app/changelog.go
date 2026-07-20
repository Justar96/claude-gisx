package app

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Claude Code keeps this file current for the installed version, so it's a
// zero-network source of "what's new".
func changelogPath() string { return filepath.Join(claudeDir(), "cache", "changelog.md") }

const (
	changelogReadLimit = 32 << 10 // newest entries are at the top; don't read 460KB
	maxWhatsNew        = 3
	maxEntryLen        = 88
)

// whatsNew returns rendered "new in <version> · <entry>" lines, newest first.
func whatsNew() []string {
	f, err := os.Open(changelogPath())
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []string
	for _, e := range parseChangelog(io.LimitReader(f, changelogReadLimit)) {
		out = append(out, dimGray+"new in "+e.version+reset+sep+dim+e.text+reset)
	}
	return out
}

type changelogEntry struct {
	version string
	text    string
}

// parseChangelog pulls the newest feature bullets out of a Claude Code
// changelog. Fixes are skipped — they don't read as news.
func parseChangelog(r io.Reader) []changelogEntry {
	var out []changelogEntry
	version := ""
	sc := bufio.NewScanner(r)
	for sc.Scan() && len(out) < maxWhatsNew {
		line := strings.TrimSpace(sc.Text())
		if v, ok := strings.CutPrefix(line, "## "); ok {
			version = v
			continue
		}
		text, ok := strings.CutPrefix(line, "- ")
		if !ok || version == "" {
			continue
		}
		if text = cleanEntry(text); text == "" {
			continue
		}
		out = append(out, changelogEntry{version: version, text: text})
	}
	return out
}

func cleanEntry(s string) string {
	s = strings.ReplaceAll(s, "`", "")
	// Links and bug-fix notes make poor one-liners.
	if strings.Contains(s, "http") || strings.HasPrefix(s, "Fixed ") {
		return ""
	}
	if len(s) > maxEntryLen {
		// Cut at the last word boundary so the line doesn't end mid-word.
		cut := strings.LastIndex(s[:maxEntryLen], " ")
		if cut < maxEntryLen/2 {
			return ""
		}
		s = s[:cut] + "…"
	}
	return s
}
