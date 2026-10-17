package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallMod(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	prev := ModFS
	ModFS = os.DirFS(filepath.Join("..", "..", "mods", "gisx"))
	t.Cleanup(func() { ModFS = prev })

	for _, want := range []string{"installed", "up to date"} {
		if got, err := installMod(); err != nil || got != want {
			t.Fatalf("installMod = %q, %v; want %q", got, err, want)
		}
	}
	dir := modDir()
	if _, err := os.Stat(filepath.Join(dir, "tests")); !os.IsNotExist(err) {
		t.Error("the tests should stay out of the installed mod")
	}
	env, _ := os.ReadFile(filepath.Join(dir, "hooks", "env.ts"))
	if !strings.Contains(string(env), "export const GISX = \"") || strings.Contains(string(env), "'claude-gisx'") {
		t.Errorf("env.ts should name this binary by path, got %q", env)
	}
	// The types Claude Code lays beside a loaded mod aren't a difference.
	types := filepath.Join(dir, ".claude-plugin", "types", "claude-code")
	if err := os.MkdirAll(types, 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(types, "index.d.ts"), []byte("x"), 0o644)
	if got, _ := installMod(); got != "up to date" {
		t.Errorf("after Claude Code wrote its types: %q, want up to date", got)
	}
	_ = os.WriteFile(filepath.Join(dir, "hooks", "register.ts"), []byte("edited"), 0o644)
	if got, _ := installMod(); got != "updated" {
		t.Errorf("after an edit: %q, want updated", got)
	}
	if ents, _ := os.ReadDir(filepath.Dir(dir)); len(ents) != 1 {
		t.Errorf("staging folders left behind: %v", ents)
	}

	if removed, err := removeMod(); !removed || err != nil {
		t.Fatalf("removeMod = %v, %v", removed, err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Error("the mod folder should be gone")
	}
}

func TestInstallModLeavesForeignPlugin(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	prev := ModFS
	ModFS = os.DirFS(filepath.Join("..", "..", "mods", "gisx"))
	t.Cleanup(func() { ModFS = prev })

	manifest := filepath.Join(modDir(), ".claude-plugin", "plugin.json")
	if err := os.MkdirAll(filepath.Dir(manifest), 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(manifest, []byte(`{"name":"gisx"}`), 0o644)
	if _, err := installMod(); err == nil {
		t.Error("a gisx plugin setup didn't write should not be replaced")
	}
	if removed, _ := removeMod(); removed {
		t.Error("nor removed")
	}
	if raw, _ := os.ReadFile(manifest); string(raw) != `{"name":"gisx"}` {
		t.Error("the foreign manifest was changed")
	}
}
