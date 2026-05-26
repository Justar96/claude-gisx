package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

func runPlugin(stdinJSON string) string {
	cmd := os.Getenv("CLAUDE_GISX_PLUGIN")
	if cmd == "" {
		return ""
	}
	timeoutSec := envInt("CLAUDE_GISX_PLUGIN_TIMEOUT", 2)
	cacheSec := envInt("CLAUDE_GISX_PLUGIN_CACHE", 30)
	cachePath := filepath.Join(cacheDir(), "statusline-plugin-cache.txt")

	if st, err := os.Stat(cachePath); err == nil &&
		time.Since(st.ModTime()) < time.Duration(cacheSec)*time.Second {
		if raw, err := os.ReadFile(cachePath); err == nil {
			return string(raw)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSec)*time.Second)
	defer cancel()

	shell, flag := shellFor()
	c := exec.CommandContext(ctx, shell, flag, cmd)
	c.Stdin = strings.NewReader(stdinJSON)
	var stdout bytes.Buffer
	c.Stdout = &stdout
	c.Stderr = nil
	if err := c.Run(); err != nil {
		// fall through to cache; new output stays empty
	}
	first := firstLine(stdout.String())
	if len(first) > 500 {
		first = first[:500]
	}
	if first != "" {
		_ = os.MkdirAll(filepath.Dir(cachePath), 0o755)
		_ = os.WriteFile(cachePath, []byte(first), 0o644)
		return first
	}
	if raw, err := os.ReadFile(cachePath); err == nil {
		return string(raw)
	}
	return ""
}

func envInt(name string, def int) int {
	v := os.Getenv(name)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return def
	}
	return n
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func shellFor() (string, string) {
	if runtime.GOOS == "windows" {
		return "cmd.exe", "/c"
	}
	return "sh", "-c"
}
