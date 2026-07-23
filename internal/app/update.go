package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const defaultRepo = "Justar96/claude-gisx"

func repoSlug() string {
	if r := os.Getenv("CLAUDE_GISX_REPO"); r != "" {
		return r
	}
	return defaultRepo
}

// ── version comparison ────────────────────────────────────────────────────

// newerThan reports whether tag is a later release than the running build.
// Both sides tolerate a leading "v"; a pre-release suffix ("-rc1") is ignored
// for ordering, so 1.2.0-rc1 and 1.2.0 compare equal.
func newerThan(tag, current string) bool {
	a, b := splitVersion(tag), splitVersion(current)
	if a == nil || b == nil {
		return false
	}
	for i := 0; i < 3; i++ {
		if a[i] != b[i] {
			return a[i] > b[i]
		}
	}
	return false
}

func splitVersion(s string) []int {
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	if i := strings.IndexAny(s, "-+"); i >= 0 {
		s = s[:i]
	}
	parts := strings.Split(s, ".")
	if len(parts) == 0 || len(parts) > 3 {
		return nil
	}
	out := make([]int, 3)
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil
		}
		out[i] = n
	}
	return out
}

// ── release lookup ────────────────────────────────────────────────────────

func latestTag(ctx context.Context, timeout time.Duration) (string, error) {
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repoSlug())
	req, err := http.NewRequestWithContext(reqCtx, "GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("github returned %d", resp.StatusCode)
	}
	var body struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body); err != nil {
		return "", err
	}
	if body.TagName == "" {
		return "", fmt.Errorf("no tag_name in release response")
	}
	return body.TagName, nil
}

type updateCache struct {
	Tag       string `json:"tag"`
	CheckedAt int64  `json:"checked_at"`
}

func updateCachePath() string { return filepath.Join(cacheDir(), "gisx-update-cache.json") }

// availableUpdate returns a newer release tag, or "" — cached so the
// statusline hits the network at most once per interval. Dev builds never
// nag: there's no meaningful version to compare against.
func availableUpdate(maxAge time.Duration) string {
	if Version == "dev" || os.Getenv("CLAUDE_GISX_NO_UPDATE_CHECK") != "" {
		return ""
	}
	tag := ""
	checked := false
	if raw, err := os.ReadFile(updateCachePath()); err == nil {
		var c updateCache
		if json.Unmarshal(raw, &c) == nil {
			if time.Since(time.Unix(c.CheckedAt, 0)) < maxAge {
				tag, checked = c.Tag, true
			}
		}
	}
	// Keyed on "did we check recently", not "do we have a tag" — an offline
	// machine has no tag to show and would otherwise re-dial GitHub on every
	// single render, paying the full timeout each time. Recording the failed
	// attempt makes the next renders fall straight through.
	if !checked {
		fetched, err := latestTag(context.Background(), 1500*time.Millisecond)
		if err != nil {
			writeUpdateCache("")
			return ""
		}
		writeUpdateCache(fetched)
		tag = fetched
	}
	if tag == "" {
		return ""
	}
	if newerThan(tag, Version) {
		return tag
	}
	return ""
}

func writeUpdateCache(tag string) {
	raw, err := json.Marshal(updateCache{Tag: tag, CheckedAt: time.Now().Unix()})
	if err != nil {
		return
	}
	_ = writeFileAtomic(updateCachePath(), raw)
}

// ── the update command ────────────────────────────────────────────────────

// assetName mirrors scripts/build.sh, which names releases x64 (not amd64).
func assetName() (string, error) {
	arch := runtime.GOARCH
	switch arch {
	case "amd64":
		arch = "x64"
	case "arm64":
	default:
		return "", fmt.Errorf("unsupported architecture: %s", runtime.GOARCH)
	}
	switch runtime.GOOS {
	case "linux":
		return "claude-gisx-linux-" + arch, nil
	case "darwin":
		if arch != "arm64" {
			return "", fmt.Errorf("Intel Macs aren't supported — build from source")
		}
		return "claude-gisx-darwin-arm64", nil
	case "windows":
		if arch != "x64" {
			return "", fmt.Errorf("unsupported architecture: %s", runtime.GOARCH)
		}
		return "claude-gisx-windows-x64.exe", nil
	}
	return "", fmt.Errorf("unsupported OS: %s", runtime.GOOS)
}

func updateCmd(opts installOpts, checkOnly bool) int {
	fmt.Println()
	fmt.Printf("  %s%sclaude-gisx%s %supdate%s\n\n", bold, blue, reset, dim, reset)

	tag, err := latestTag(context.Background(), 10*time.Second)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  %s could not reach GitHub: %v\n\n", failMark, err)
		return 3
	}
	writeUpdateCache(tag)
	fmt.Printf("  installed  %s%s%s\n", bold, Version, reset)
	fmt.Printf("  latest     %s%s%s\n\n", bold, tag, reset)

	upToDate := !newerThan(tag, Version)
	if checkOnly {
		if upToDate {
			fmt.Printf("  %s up to date\n\n", okMark)
		} else {
			fmt.Printf("  %s%s available%s — run %sclaude-gisx update%s\n\n", green, tag, reset, cyan, reset)
		}
		return 0
	}
	if upToDate && !opts.force {
		fmt.Printf("  %s already up to date %s(--force to reinstall)%s\n\n", okMark, dim, reset)
		return 0
	}

	asset, err := assetName()
	if err != nil {
		fmt.Fprintf(os.Stderr, "  %s %v\n\n", failMark, err)
		return 2
	}
	self, err := selfPath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "  %s cannot locate the running binary: %v\n\n", failMark, err)
		return 1
	}

	fmt.Printf("  %sdownloading%s %s\n", dim, reset, asset)
	bin, err := downloadAsset(tag, asset)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  %s download failed: %v\n\n", failMark, err)
		return 3
	}
	if err := verifyChecksum(tag, asset, bin); err != nil {
		fmt.Fprintf(os.Stderr, "  %s %v\n\n", failMark, err)
		return 3
	}
	fmt.Printf("  %s checksum verified\n", okMark)

	if err := replaceSelf(self, bin); err != nil {
		fmt.Fprintf(os.Stderr, "  %s install failed: %v\n", failMark, err)
		fmt.Fprintf(os.Stderr, "  %sno write access to %s? re-run the installer instead%s\n\n", dim, self, reset)
		return 1
	}
	fmt.Printf("  %s updated to %s%s%s %s(%s)%s\n", okMark, bold, tag, reset, dim, self, reset)
	fmt.Printf("  %srestart Claude Code to pick it up%s\n\n", dim, reset)
	return 0
}

// selfPath resolves symlinks so an update replaces the real binary rather
// than clobbering a symlink that points at it.
func selfPath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		return exe, nil
	}
	return resolved, nil
}

func downloadAsset(tag, asset string) ([]byte, error) {
	url := fmt.Sprintf("https://github.com/%s/releases/download/%s/%s", repoSlug(), tag, asset)
	return httpGet(url, 120*time.Second, 128<<20)
}

// verifyChecksum fails closed: a release without SHA256SUMS, or one that
// doesn't list this asset, aborts the update.
func verifyChecksum(tag, asset string, bin []byte) error {
	url := fmt.Sprintf("https://github.com/%s/releases/download/%s/SHA256SUMS", repoSlug(), tag)
	sums, err := httpGet(url, 30*time.Second, 1<<20)
	if err != nil {
		return fmt.Errorf("no SHA256SUMS published for %s: %v", tag, err)
	}
	want := checksumFor(string(sums), asset)
	if want == "" {
		return fmt.Errorf("SHA256SUMS has no entry for %s", asset)
	}
	sum := sha256.Sum256(bin)
	if got := hex.EncodeToString(sum[:]); got != want {
		return fmt.Errorf("checksum mismatch: got %s, want %s", got, want)
	}
	return nil
}

// checksumFor reads "<hash>  <file>" lines as produced by sha256sum.
func checksumFor(sums, asset string) string {
	for _, line := range strings.Split(sums, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == asset {
			return fields[0]
		}
	}
	return ""
}

func httpGet(url string, timeout time.Duration, maxBytes int64) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("%s returned %d", url, resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxBytes))
}

// replaceSelf swaps the binary atomically via a same-directory temp file, so
// a failed write never leaves a half-written binary in place.
func replaceSelf(self string, bin []byte) error {
	tmp := self + ".new"
	if err := os.WriteFile(tmp, bin, 0o755); err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		// Windows won't let you overwrite a running image, but it will let
		// you rename it out of the way first.
		old := self + ".old"
		_ = os.Remove(old)
		if err := os.Rename(self, old); err != nil {
			_ = os.Remove(tmp)
			return err
		}
		if err := os.Rename(tmp, self); err != nil {
			_ = os.Rename(old, self)
			return err
		}
		return nil
	}
	if err := os.Rename(tmp, self); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
