package app

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Line 2 is the one part of the statusline whose data doesn't come from the
// payload alone, so it's the one that breaks when a session restarts:
// Claude Code omits `rate_limits` until the session's first API response, and
// the OAuth cache used to live in a temp dir that a reboot wipes. Both meant a
// blank or half-drawn usage line for the first minutes after reopening.
// Everything here backs that line with state that survives close/reopen.

// Durable, unlike os.TempDir() — line 2's numbers stay true across reboots.
func lineCacheDir() string { return filepath.Join(claudeDir(), "cache") }

func limitsCachePath() string { return filepath.Join(lineCacheDir(), "statusline-limits.json") }

type limitsSnapshot struct {
	FiveHour   *rateBucket `json:"five_hour,omitempty"`
	SevenDay   *rateBucket `json:"seven_day,omitempty"`
	SpendLimit *rateBucket `json:"spend_limit,omitempty"`
}

func hasBuckets(rl *rateLimits) bool {
	return rl != nil && (rl.FiveHour != nil || rl.SevenDay != nil || rl.SpendLimit != nil)
}

func saveLimits(rl *rateLimits) {
	if !hasBuckets(rl) {
		return
	}
	raw, err := json.Marshal(limitsSnapshot{FiveHour: rl.FiveHour, SevenDay: rl.SevenDay, SpendLimit: rl.SpendLimit})
	if err != nil {
		return
	}
	// Renders are constant but these numbers only move when you spend tokens.
	// Comparing a ~150-byte file beats a create/write/rename every redraw.
	if old, err := os.ReadFile(limitsCachePath()); err == nil && bytes.Equal(old, raw) {
		return
	}
	_ = writeFileAtomic(limitsCachePath(), raw)
}

// loadLimits returns the last-known buckets for a session whose payload hasn't
// carried any yet. A bucket is only reused while its window is still open —
// once resets_at passes, the quota has rolled over and the old percentage is a
// lie, so it's dropped rather than shown. Buckets with no resets_at can't be
// checked that way and are never reused.
func loadLimits(now time.Time) *rateLimits {
	raw, err := os.ReadFile(limitsCachePath())
	if err != nil {
		return nil
	}
	var s limitsSnapshot
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil
	}
	open := func(b *rateBucket) *rateBucket {
		if b == nil || b.ResetsAt <= now.Unix() {
			return nil
		}
		return b
	}
	rl := &rateLimits{FiveHour: open(s.FiveHour), SevenDay: open(s.SevenDay), SpendLimit: open(s.SpendLimit)}
	if !hasBuckets(rl) {
		return nil
	}
	return rl
}

// writeFileAtomic keeps a cache readable even if the writer dies mid-write —
// which is exactly what closing Claude Code does to an in-flight statusline
// render. A torn file would fail to parse and cost the next session a blocking
// network fetch to rebuild what it already had.
func writeFileAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	sweepStaleTemps(filepath.Dir(path), filepath.Base(path))
	return nil
}

// A render killed between the write and the rename leaves its temp file
// behind for good, and this process gets killed routinely — that's the whole
// reason the write is atomic. Sweep the strays, but only ones old enough that
// no in-flight write from a concurrent session could still own them.
const tempTTL = 10 * time.Minute

func sweepStaleTemps(dir, base string) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range ents {
		name := e.Name()
		if e.IsDir() || !strings.HasPrefix(name, base+".") {
			continue
		}
		info, err := e.Info()
		if err != nil || time.Since(info.ModTime()) < tempTTL {
			continue
		}
		_ = os.Remove(filepath.Join(dir, name))
	}
}
