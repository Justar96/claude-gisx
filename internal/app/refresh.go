package app

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// The statusline process exits as soon as it prints, so a network refresh
// can't run in a goroutine behind it. Instead a render that finds a stale
// cache shows the cached value and starts `claude-gisx refresh <kind>` as a
// detached child; the child does the slow fetch and writes the cache the
// next render reads. No render ever waits on the network.

const (
	refreshTimeout = 10 * time.Second
	// A lock older than this belongs to a child that died without cleaning
	// up; take it over rather than never refreshing again.
	refreshLockTTL = 30 * time.Second
)

// startRefresh is swapped out in tests, where os.Executable is the test
// binary and spawning it would rerun the suite.
var startRefresh = spawnRefresh

func refreshLockPath(kind string) string {
	return filepath.Join(lineCacheDir(), "statusline-refresh-"+kind+".lock")
}

// spawnRefresh starts one detached refresh per kind; renders that arrive
// while it is in flight see the lock and skip.
func spawnRefresh(kind string) {
	lock := refreshLockPath(kind)
	if !claimLock(lock) {
		return
	}
	self, err := os.Executable()
	if err != nil {
		_ = os.Remove(lock)
		return
	}
	cmd := exec.Command(self, "refresh", kind)
	cmd.SysProcAttr = detachedAttr() // nil stdio is /dev/null, so Claude Code's pipe closes now
	if err := cmd.Start(); err != nil {
		_ = os.Remove(lock)
		return
	}
	_ = cmd.Process.Release()
}

func claimLock(p string) bool {
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return false
	}
	for i := 0; i < 2; i++ {
		f, err := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			f.Close()
			return true
		}
		if !errors.Is(err, os.ErrExist) {
			return false
		}
		st, err := os.Stat(p)
		if err != nil || time.Since(st.ModTime()) < refreshLockTTL {
			return false
		}
		_ = os.Remove(p)
	}
	return false
}

// refreshCmd is the detached child's entry point.
func refreshCmd(kind string) int {
	defer os.Remove(refreshLockPath(kind))
	ctx, cancel := context.WithTimeout(context.Background(), refreshTimeout)
	defer cancel()
	switch kind {
	case "usage":
		doOAuthFetch(ctx, usageCachePath())
	case "update":
		tag, err := latestTag(ctx, refreshTimeout)
		if err != nil {
			tag = ""
		}
		writeUpdateCache(tag)
	default:
		return 1
	}
	return 0
}
