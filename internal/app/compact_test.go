package app

import "testing"

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
	// All env vars cleared by default in each subtest via t.Setenv.

	t.Run("no env vars → no badge", func(t *testing.T) {
		clearCompactEnv(t)
		got := computeCompactState(200_000)
		if got.userSet || got.off || got.pct != 0 {
			t.Errorf("expected zero-value state, got %+v", got)
		}
	})

	t.Run("DISABLE_COMPACT=1 → off", func(t *testing.T) {
		clearCompactEnv(t)
		t.Setenv("DISABLE_COMPACT", "1")
		got := computeCompactState(200_000)
		if !got.off {
			t.Errorf("expected off=true, got %+v", got)
		}
	})

	t.Run("DISABLE_COMPACT=0 → ignored", func(t *testing.T) {
		clearCompactEnv(t)
		t.Setenv("DISABLE_COMPACT", "0")
		got := computeCompactState(200_000)
		if got.off {
			t.Errorf("expected off=false for falsy value, got %+v", got)
		}
	})

	t.Run("CLAUDE_GISX_COMPACT_PCT=70 → pct=70 userSet", func(t *testing.T) {
		clearCompactEnv(t)
		t.Setenv("CLAUDE_GISX_COMPACT_PCT", "70")
		got := computeCompactState(200_000)
		if got.pct != 70 || !got.userSet {
			t.Errorf("got %+v, want pct=70 userSet=true", got)
		}
	})

	t.Run("CLAUDE_AUTOCOMPACT_PCT_OVERRIDE=80 → pct=80", func(t *testing.T) {
		clearCompactEnv(t)
		t.Setenv("CLAUDE_AUTOCOMPACT_PCT_OVERRIDE", "80")
		got := computeCompactState(200_000)
		if got.pct != 80 || !got.userSet {
			t.Errorf("got %+v, want pct=80 userSet=true", got)
		}
	})

	t.Run("WINDOW remaps onto bar scale", func(t *testing.T) {
		clearCompactEnv(t)
		t.Setenv("CLAUDE_CODE_AUTO_COMPACT_WINDOW", "500000")
		// Default pct=95, window=500k, ctxSize=1M → 95 * 500k / 1M = 47
		got := computeCompactState(1_000_000)
		if got.pct != 47 || !got.userSet {
			t.Errorf("got %+v, want pct=47 userSet=true", got)
		}
	})

	t.Run("WINDOW with explicit pct", func(t *testing.T) {
		clearCompactEnv(t)
		t.Setenv("CLAUDE_AUTOCOMPACT_PCT_OVERRIDE", "80")
		t.Setenv("CLAUDE_CODE_AUTO_COMPACT_WINDOW", "500000")
		// 80 * 500k / 1M = 40
		got := computeCompactState(1_000_000)
		if got.pct != 40 {
			t.Errorf("got %+v, want pct=40", got)
		}
	})

	t.Run("WINDOW >= ctxSize → no remap", func(t *testing.T) {
		clearCompactEnv(t)
		t.Setenv("CLAUDE_AUTOCOMPACT_PCT_OVERRIDE", "80")
		t.Setenv("CLAUDE_CODE_AUTO_COMPACT_WINDOW", "200000")
		got := computeCompactState(200_000)
		if got.pct != 80 {
			t.Errorf("got %+v, want pct=80", got)
		}
	})

	t.Run("DISABLE_COMPACT wins over everything", func(t *testing.T) {
		clearCompactEnv(t)
		t.Setenv("DISABLE_COMPACT", "1")
		t.Setenv("CLAUDE_AUTOCOMPACT_PCT_OVERRIDE", "80")
		t.Setenv("CLAUDE_GISX_COMPACT_PCT", "60")
		got := computeCompactState(200_000)
		if !got.off || got.pct != 0 || got.userSet {
			t.Errorf("got %+v, want off=true and nothing else", got)
		}
	})
}

func clearCompactEnv(t *testing.T) {
	t.Helper()
	t.Setenv("DISABLE_COMPACT", "")
	t.Setenv("CLAUDE_GISX_COMPACT_PCT", "")
	t.Setenv("CLAUDE_AUTOCOMPACT_PCT_OVERRIDE", "")
	t.Setenv("CLAUDE_CODE_AUTO_COMPACT_WINDOW", "")
}
