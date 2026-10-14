package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsDisableFalsy(t *testing.T) {
	for _, s := range []string{"", "0", "false", "FALSE", "False"} {
		if !isDisableFalsy(s) {
			t.Errorf("isDisableFalsy(%q) = false, want true", s)
		}
	}
	for _, s := range []string{"1", "true", "yes", "on"} {
		if isDisableFalsy(s) {
			t.Errorf("isDisableFalsy(%q) = true, want false", s)
		}
	}
}

func TestComputeCompactState(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		ctx  int
		want compactState
	}{
		{"nothing set → raw window", nil, 200_000, compactState{}},
		{"DISABLE_COMPACT=1 → off", map[string]string{"DISABLE_COMPACT": "1"}, 200_000, compactState{off: true}},
		{"DISABLE_AUTO_COMPACT=1 → off", map[string]string{"DISABLE_AUTO_COMPACT": "1"}, 200_000, compactState{off: true}},
		{"DISABLE_COMPACT=0 → ignored", map[string]string{"DISABLE_COMPACT": "0"}, 200_000, compactState{}},
		// The case that started this: a 360k window on a 1M model.
		{"window → window − 13k", map[string]string{"CLAUDE_CODE_AUTO_COMPACT_WINDOW": "360000"}, 1_000_000, compactState{threshold: 347_000}},
		{"window capped at the model", map[string]string{"CLAUDE_CODE_AUTO_COMPACT_WINDOW": "500000"}, 200_000, compactState{threshold: 187_000}},
		{"window floored at 100k", map[string]string{"CLAUDE_CODE_AUTO_COMPACT_WINDOW": "50000"}, 1_000_000, compactState{threshold: 87_000}},
		{"pct alone applies to the model window", map[string]string{"CLAUDE_AUTOCOMPACT_PCT_OVERRIDE": "80"}, 200_000, compactState{threshold: 160_000}},
		{"pct and window", map[string]string{"CLAUDE_AUTOCOMPACT_PCT_OVERRIDE": "50", "CLAUDE_CODE_AUTO_COMPACT_WINDOW": "400000"}, 1_000_000, compactState{threshold: 200_000}},
		{"pct never past the buffer", map[string]string{"CLAUDE_AUTOCOMPACT_PCT_OVERRIDE": "100"}, 200_000, compactState{threshold: 187_000}},
		{"DISABLE wins over everything", map[string]string{"DISABLE_COMPACT": "1", "CLAUDE_AUTOCOMPACT_PCT_OVERRIDE": "80"}, 200_000, compactState{off: true}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			clearCompactEnv(t)
			t.Setenv("HOME", t.TempDir())
			for k, v := range c.env {
				t.Setenv(k, v)
			}
			if got := computeCompactState(c.ctx, "claude-opus-5-5", ""); got != c.want {
				t.Errorf("got %+v, want %+v", got, c.want)
			}
		})
	}
}

// /autocompact writes the window into settings rather than the environment.
func TestCompactWindowFromSettings(t *testing.T) {
	clearCompactEnv(t)
	home, project := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	write := func(path, body string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(home, ".claude", "settings.json"),
		`{"autoCompactWindow": 400000, "modelSettings": {"claude-opus-5-5": {"autoCompactWindow": 300000}}}`)

	if got := computeCompactState(1_000_000, "claude-opus-5-5[1m]", project); got.threshold != 287_000 {
		t.Errorf("per-model window: got %+v, want 287000", got)
	}
	if got := computeCompactState(1_000_000, "claude-sonnet-5-5", project); got.threshold != 387_000 {
		t.Errorf("top-level window: got %+v, want 387000", got)
	}
	// A later file's top-level value replaces the earlier per-model one.
	write(filepath.Join(project, ".claude", "settings.local.json"), `{"autoCompactWindow": 500000}`)
	if got := computeCompactState(1_000_000, "claude-opus-5-5", project); got.threshold != 487_000 {
		t.Errorf("local override: got %+v, want 487000", got)
	}
	// "auto" hands it back to Claude Code's tuned window.
	write(filepath.Join(project, ".claude", "settings.local.json"), `{"autoCompactWindow": "auto"}`)
	if got := computeCompactState(1_000_000, "claude-opus-5-5", project); got.threshold != 0 {
		t.Errorf("auto: got %+v, want 0", got)
	}
	// The environment beats settings.
	t.Setenv("CLAUDE_CODE_AUTO_COMPACT_WINDOW", "360000")
	if got := computeCompactState(1_000_000, "claude-opus-5-5", project); got.threshold != 347_000 {
		t.Errorf("env over settings: got %+v, want 347000", got)
	}
}

func TestCanonicalModel(t *testing.T) {
	for in, want := range map[string]string{
		"claude-opus-5-5":          "claude-opus-5-5",
		"claude-opus-5-5[1m]":      "claude-opus-5-5",
		"Claude-Opus-5-5-20261001": "claude-opus-5-5",
		"claude-opus-5-5@20261001": "claude-opus-5-5",
	} {
		if got := canonicalModel(in); got != want {
			t.Errorf("canonicalModel(%q) = %q, want %q", in, got, want)
		}
	}
}

func clearCompactEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{"DISABLE_COMPACT", "DISABLE_AUTO_COMPACT",
		"CLAUDE_AUTOCOMPACT_PCT_OVERRIDE", "CLAUDE_CODE_AUTO_COMPACT_WINDOW"} {
		t.Setenv(k, "")
	}
}
