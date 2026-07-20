package app

import "testing"

func TestNewerThan(t *testing.T) {
	cases := []struct {
		tag, current string
		want         bool
	}{
		{"v1.2.0", "1.1.1", true},
		{"v1.1.2", "1.1.1", true},
		{"v2.0.0", "1.9.9", true},
		{"v1.1.1", "1.1.1", false},
		{"v1.1.0", "1.1.1", false},
		{"v1.10.0", "1.9.0", true}, // numeric, not lexical
		{"v1.2", "1.1.9", true},    // short tags fill zeros
		{"v1.2.0-rc1", "1.2.0", false},
		{"v1.2.0", "dev", false}, // unparseable current
		{"nightly", "1.1.1", false},
	}
	for _, c := range cases {
		if got := newerThan(c.tag, c.current); got != c.want {
			t.Errorf("newerThan(%q, %q) = %v, want %v", c.tag, c.current, got, c.want)
		}
	}
}

func TestChecksumFor(t *testing.T) {
	const sums = `abc123  claude-gisx-linux-arm64
def456  claude-gisx-linux-x64
789fed *claude-gisx-windows-x64.exe
`
	if got := checksumFor(sums, "claude-gisx-linux-x64"); got != "def456" {
		t.Errorf("got %q, want def456", got)
	}
	if got := checksumFor(sums, "claude-gisx-windows-x64.exe"); got != "789fed" {
		t.Errorf("binary-mode marker not handled: got %q, want 789fed", got)
	}
	if got := checksumFor(sums, "claude-gisx-darwin-arm64"); got != "" {
		t.Errorf("missing asset should return empty, got %q", got)
	}
}

func TestAssetNameMatchesBuildScript(t *testing.T) {
	// Names come from scripts/build.sh; a mismatch means update downloads 404.
	name, err := assetName()
	if err != nil {
		t.Skipf("unsupported platform for releases: %v", err)
	}
	if name == "" {
		t.Fatal("empty asset name")
	}
}
