package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIdleCompactAt(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv("CLAUDE_CODE_IDLE_COMPACT_MIN_TOKENS", "")
	project := t.TempDir()
	writeGlobal := func(body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(home, ".claude.json"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	const exp = int64(10_000)
	warm := &promptCache{Warm: true, CachingObserved: true, TTL: "1h", ExpiresAt: exp}
	on := `{"cachedGrowthBookFeatures":{"tengu_sunny_locket":{"mode":"compact"}}}`

	writeGlobal(on)
	if got, want := idleCompactAt(warm, 250_000, compactState{}, project), exp-360; got != want {
		t.Errorf("armed = %d, want %d (54 minutes into the hour)", got, want)
	}
	if got := idleCompactAt(warm, 150_000, compactState{}, project); got != 0 {
		t.Errorf("under the 200k default = %d, want 0", got)
	}
	five := *warm
	five.TTL = "5m"
	if got := idleCompactAt(&five, 250_000, compactState{}, project); got != 0 {
		t.Errorf("5m cache = %d, want 0", got)
	}
	if got := idleCompactAt(warm, 250_000, compactState{off: true}, project); got != 0 {
		t.Errorf("auto-compact off = %d, want 0", got)
	}

	t.Setenv("CLAUDE_CODE_IDLE_COMPACT_MIN_TOKENS", "50000") // floored at 100k
	if got := idleCompactAt(warm, 120_000, compactState{}, project); got != exp-360 {
		t.Errorf("env threshold = %d, want %d", got, exp-360)
	}
	t.Setenv("CLAUDE_CODE_IDLE_COMPACT_MIN_TOKENS", "")

	writeGlobal(`{"cachedGrowthBookFeatures":{"tengu_sunny_locket":{"mode":"compact","fireAtFraction":0.99}}}`)
	if got := idleCompactAt(warm, 250_000, compactState{}, project); got != exp-180 {
		t.Errorf("fraction clamped to 0.95 = %d, want %d", got, exp-180)
	}

	for name, body := range map[string]string{
		"not rolled out":   `{"cachedGrowthBookFeatures":{"tengu_sunny_locket":{"mode":"off"}}}`,
		"log only":         `{"cachedGrowthBookFeatures":{"tengu_sunny_locket":{"mode":"log"}}}`,
		"no cached flag":   `{}`,
		"autoCompact off":  `{"autoCompactEnabled":false,"cachedGrowthBookFeatures":{"tengu_sunny_locket":{"mode":"compact"}}}`,
		"unreadable state": `not json`,
	} {
		writeGlobal(body)
		if got := idleCompactAt(warm, 250_000, compactState{}, project); got != 0 {
			t.Errorf("%s = %d, want 0", name, got)
		}
	}

	writeGlobal(on)
	local := filepath.Join(project, ".claude", "settings.local.json")
	if err := os.MkdirAll(filepath.Dir(local), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(local, []byte(`{"idleCompaction":false}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := idleCompactAt(warm, 250_000, compactState{}, project); got != 0 {
		t.Errorf("idleCompaction: false = %d, want 0", got)
	}
}
