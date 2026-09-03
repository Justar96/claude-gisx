package app

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// entry writes one assistant line in the transcript's shape. Two entries with
// the same id are how Claude Code records a single turn split across blocks.
func entry(id string, in, write, read, out int64) string {
	return fmt.Sprintf(
		`{"type":"assistant","message":{"id":%q,"usage":{"input_tokens":%d,"cache_creation_input_tokens":%d,"cache_read_input_tokens":%d,"output_tokens":%d}}}`+"\n",
		id, in, write, read, out)
}

func transcriptFixture(t *testing.T, lines ...string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "b1dffdf7.jsonl")
	if err := os.WriteFile(p, []byte(strings.Join(lines, "")), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func appendLines(t *testing.T, path string, lines ...string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(strings.Join(lines, "")); err != nil {
		t.Fatal(err)
	}
}

func TestSessionTokensSumsAndDeduplicates(t *testing.T) {
	isolateHome(t)
	p := transcriptFixture(t,
		`{"type":"user","message":{"content":"hi"}}`+"\n",
		entry("msg_a", 2, 100, 900, 50),
		entry("msg_a", 2, 100, 900, 50), // same turn, second block
		entry("msg_b", 3, 200, 1800, 70),
	)

	got := sessionTokens(p)
	want := tokenTotals{Input: 5, Write: 300, Read: 2700, Output: 120}
	if got != want {
		t.Fatalf("totals = %+v, want %+v", got, want)
	}
}

// The offset must land past the last complete line only, so the half-written
// append is picked up on the next render rather than lost.
func TestSessionTokensResumesFromOffset(t *testing.T) {
	isolateHome(t)
	p := transcriptFixture(t, entry("msg_a", 0, 100, 900, 50))

	if got := sessionTokens(p); got.Read != 900 {
		t.Fatalf("first pass read = %d, want 900", got.Read)
	}
	partial := entry("msg_b", 0, 200, 1800, 70)
	appendLines(t, p, partial[:len(partial)/2])
	if got := sessionTokens(p); got.Read != 900 {
		t.Fatalf("partial line counted: read = %d, want 900", got.Read)
	}
	appendLines(t, p, partial[len(partial)/2:])
	if got := sessionTokens(p); got.Read != 2700 {
		t.Fatalf("second pass read = %d, want 2700", got.Read)
	}
	if _, err := os.Stat(tokenCachePath(p)); err != nil {
		t.Fatal("a cache file should have been written")
	}
}

// A file shorter than the recorded offset isn't the one that was summed.
func TestSessionTokensRestartsOnTruncation(t *testing.T) {
	isolateHome(t)
	p := transcriptFixture(t, entry("msg_a", 0, 100, 900, 50), entry("msg_b", 0, 200, 1800, 70))
	sessionTokens(p)

	if err := os.WriteFile(p, []byte(entry("msg_c", 0, 10, 90, 5)), 0o644); err != nil {
		t.Fatal(err)
	}
	got := sessionTokens(p)
	want := tokenTotals{Write: 10, Read: 90, Output: 5}
	if got != want {
		t.Fatalf("totals = %+v, want %+v", got, want)
	}
}

func TestSessionTokensMissingTranscript(t *testing.T) {
	isolateHome(t)
	if got := sessionTokens(""); got != (tokenTotals{}) {
		t.Errorf("empty path = %+v, want zero", got)
	}
	if got := sessionTokens(filepath.Join(t.TempDir(), "gone.jsonl")); got != (tokenTotals{}) {
		t.Errorf("missing file = %+v, want zero", got)
	}
}

// Strips the escapes rather than calling disableColor, which blanks the color
// globals for every test that runs after it.
var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func TestPaintTokens(t *testing.T) {
	if got := paintTokens(tokenTotals{}, nil); got != "" {
		t.Errorf("no input should render nothing, got %q", got)
	}
	got := ansiRe.ReplaceAllString(paintTokens(tokenTotals{Input: 2_000, Write: 8_000, Read: 990_000, Output: 34_000}, nil), "")
	want := "cache 99% · in 1.0M · out 34.0k"
	if got != want {
		t.Errorf("paintTokens = %q, want %q", got, want)
	}
}

// With prompt_cache in the payload, Claude Code's own hit ratio is the one to
// show — it covers exactly the main conversation — while the transcript still
// supplies the in/out totals the payload lacks.
func TestPaintTokensPrefersPayloadCache(t *testing.T) {
	ratio := 0.42
	pc := &promptCache{Warm: false, CachingObserved: true, HitRatio: &ratio, Misses: 3}
	got := ansiRe.ReplaceAllString(paintTokens(tokenTotals{Input: 500, Read: 500, Output: 10}, pc), "")
	want := "cache 42% cold 3 miss · in 1.0k · out 10"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	// A payload ratio alone (no transcript yet) still renders the rate.
	got = ansiRe.ReplaceAllString(paintTokens(tokenTotals{}, &promptCache{Warm: true, CachingObserved: true, HitRatio: &ratio}), "")
	if got != "cache 42%" {
		t.Errorf("payload-only: got %q", got)
	}
	// A null hit_ratio with an empty transcript has nothing to say.
	if got := paintTokens(tokenTotals{}, &promptCache{}); got != "" {
		t.Errorf("null ratio: got %q, want empty", got)
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("short", 10); got != "short" {
		t.Errorf("got %q", got)
	}
	if got := truncate("a fairly long session title", 10); got != "a fairly…" {
		t.Errorf("got %q", got)
	}
}
