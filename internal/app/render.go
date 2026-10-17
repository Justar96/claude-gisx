package app

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type sessionInput struct {
	Model         *modelInfo     `json:"model,omitempty"`
	Workspace     *workspaceInfo `json:"workspace,omitempty"`
	Cwd           string         `json:"cwd,omitempty"`
	ContextWindow *contextInfo   `json:"context_window,omitempty"`
	Cost          *costInfo      `json:"cost,omitempty"`
	Effort        *levelInfo     `json:"effort,omitempty"`
	FastMode      bool           `json:"fast_mode,omitempty"`
	OutputStyle   *nameInfo      `json:"output_style,omitempty"`
	Vim           *modeInfo      `json:"vim,omitempty"`
	Agent         *nameInfo      `json:"agent,omitempty"`
	PR            *prInfo        `json:"pr,omitempty"`
	Worktree      *nameInfo      `json:"worktree,omitempty"`
	RateLimits    *rateLimits    `json:"rate_limits,omitempty"`
	PromptCache   *promptCache   `json:"prompt_cache,omitempty"`
	SessionID     string         `json:"session_id,omitempty"`
	Transcript    string         `json:"transcript_path,omitempty"`
}

type modelInfo struct {
	ID          string `json:"id,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
}
type workspaceInfo struct {
	CurrentDir string `json:"current_dir,omitempty"`
	ProjectDir string `json:"project_dir,omitempty"`
	// Any linked worktree, not just a Claude worktree session (worktree.name).
	GitWorktree string `json:"git_worktree,omitempty"`
}
type contextInfo struct {
	UsedPercentage    *float64 `json:"used_percentage,omitempty"`
	ContextWindowSize int      `json:"context_window_size,omitempty"`
}
type costInfo struct {
	TotalCostUSD    float64 `json:"total_cost_usd,omitempty"`
	TotalDurationMS int64   `json:"total_duration_ms,omitempty"`
	LinesAdded      int64   `json:"total_lines_added,omitempty"`
	LinesRemoved    int64   `json:"total_lines_removed,omitempty"`
}
type levelInfo struct {
	Level string `json:"level,omitempty"`
}
type nameInfo struct {
	Name string `json:"name,omitempty"`
}
type modeInfo struct {
	Mode string `json:"mode,omitempty"`
}
type prInfo struct {
	Number      int    `json:"number,omitempty"`
	URL         string `json:"url,omitempty"`
	ReviewState string `json:"review_state,omitempty"`
	Kind        string `json:"kind,omitempty"` // "mr" for a GitLab merge request
}
type rateLimits struct {
	FiveHour *rateBucket `json:"five_hour,omitempty"`
	SevenDay *rateBucket `json:"seven_day,omitempty"`
	// Behind a Claude apps gateway; used_percentage may exceed 100.
	SpendLimit *rateBucket `json:"spend_limit,omitempty"`
}
type rateBucket struct {
	UsedPercentage float64 `json:"used_percentage,omitempty"`
	ResetsAt       int64   `json:"resets_at,omitempty"`
}

// Claude Code's own view of the main conversation's cache (v2.1.251+). It
// covers exactly the requests that count, so it wins over our transcript sum
// for the hit rate; the transcript still supplies the in/out totals.
type promptCache struct {
	Warm            bool     `json:"warm"`             // cached prefix still within its TTL
	CachingObserved bool     `json:"caching_observed"` // false: caching off or not reported
	TTL             string   `json:"ttl"`              // "5m" or "1h"
	ExpiresAt       int64    `json:"expires_at"`       // when the prefix goes cold
	HitRatio        *float64 `json:"hit_ratio"`        // null until any input was counted
	Misses          int      `json:"misses"`
	RecacheIfCold   *int64   `json:"recache_tokens_if_cold"`
}

func renderStatusline(stdinJSON string) {
	if strings.TrimSpace(stdinJSON) == "" {
		fmt.Print("Claude")
		return
	}
	var data sessionInput
	if err := json.Unmarshal([]byte(stdinJSON), &data); err != nil {
		fmt.Print("Claude")
		return
	}

	modelName := "Claude"
	if data.Model != nil && data.Model.DisplayName != "" {
		modelName = shortModelName(data.Model.DisplayName)
	}
	pctUsed := 0
	if data.ContextWindow != nil && data.ContextWindow.UsedPercentage != nil {
		pctUsed = int(*data.ContextWindow.UsedPercentage)
	}
	ctxSize := 200_000
	if data.ContextWindow != nil && data.ContextWindow.ContextWindowSize > 0 {
		ctxSize = data.ContextWindow.ContextWindowSize
	}
	cwd := ""
	switch {
	case data.Workspace != nil && data.Workspace.CurrentDir != "":
		cwd = data.Workspace.CurrentDir
	case data.Workspace != nil && data.Workspace.ProjectDir != "":
		cwd = data.Workspace.ProjectDir
	case data.Cwd != "":
		cwd = data.Cwd
	default:
		cwd, _ = os.Getwd()
	}
	durationMs := int64(0)
	costUsd := 0.0
	if data.Cost != nil {
		durationMs = data.Cost.TotalDurationMS
		costUsd = data.Cost.TotalCostUSD
	}
	// `effort` is only sent for models that support it — absent means the
	// model has no effort dial, so don't invent one from settings.
	effortLevel := ""
	if data.Effort != nil {
		effortLevel = data.Effort.Level
	}

	// Everything that leaves the process — git, the OAuth endpoint, the
	// transcript, the plugin, the release check — is independent, so the
	// render pays for the slowest one rather than the sum. Claude Code kills
	// a render that overruns the next trigger, so wall time is the budget.
	var (
		g         gitState
		u         *usage
		tk        tokenTotals
		pluginOut string
		updateTag string
		wg        sync.WaitGroup
	)
	wg.Add(5)
	go func() { defer wg.Done(); g = gitInfo(cwd) }()
	go func() { defer wg.Done(); u = fetchUsage() }()
	go func() { defer wg.Done(); tk = sessionTokens(data.Transcript) }()
	go func() { defer wg.Done(); pluginOut = runPlugin(stdinJSON) }()
	go func() { defer wg.Done(); updateTag = availableUpdate(6 * time.Hour) }()

	modelID, projectDir := "", cwd
	if data.Model != nil {
		modelID = data.Model.ID
	}
	if data.Workspace != nil && data.Workspace.ProjectDir != "" {
		projectDir = data.Workspace.ProjectDir
	}
	cs := computeCompactState(ctxSize, modelID, projectDir)
	// The gisx mod's copy of /context, when it's installed: the window by
	// category, and the point auto-compact fires as Claude Code itself works
	// it out. That beats our reading of the settings — the buffer it keeps
	// below the window has changed between releases — and covers a window
	// Claude Code tunes itself, which settings can't.
	cm := loadCtxMap(data.SessionID)
	if cm != nil && !cs.off && cm.AutoCompactThreshold != nil && *cm.AutoCompactThreshold > 0 {
		cs.threshold = int(*cm.AutoCompactThreshold)
	}
	// The prefix a cold cache would rewrite is the prompt idle compaction
	// measures; before Claude Code reports it, the context in use stands in.
	var prompt int64
	if pc := data.PromptCache; pc != nil && pc.RecacheIfCold != nil && *pc.RecacheIfCold > 0 {
		prompt = *pc.RecacheIfCold
	} else if data.ContextWindow != nil && data.ContextWindow.UsedPercentage != nil {
		prompt = int64(*data.ContextWindow.UsedPercentage / 100 * float64(ctxSize))
	}
	idleAt := idleCompactAt(data.PromptCache, prompt, cs, projectDir)
	// With auto-compact configured, the bar measures the runway to the
	// compaction point rather than the model's full window: 100% is where
	// Claude Code compacts, the same denominator its own "% left" uses.
	barPct, barLabel := pctUsed, fmtCtxLabel(ctxSize)
	if cs.threshold > 0 && data.ContextWindow != nil && data.ContextWindow.UsedPercentage != nil {
		used := *data.ContextWindow.UsedPercentage / 100 * float64(ctxSize)
		barPct, barLabel = int(used*100/float64(cs.threshold)), fmtCtxLabel(cs.threshold)
	}
	dir := filepath.Base(cwd)
	dur := fmtDuration(durationMs)
	trend := tokenTrend()
	wg.Wait()

	// ── Line 1: model + bar + workspace + session meta ────────────────────
	var L strings.Builder
	L.WriteString(bold + blue + modelName + reset)
	if effortLevel != "" {
		L.WriteString(dim + "/" + reset + effortColor(effortLevel) + effortLevel + reset)
	}
	if data.FastMode {
		// A word, not an emoji — emoji widths vary per terminal and shift the line.
		L.WriteString(sep + yellow + "fast" + reset)
	}
	// The fill's length is the live payload's; its colors are the last
	// breakdown the mod sent, a turn behind at most. The percentage keeps its
	// threshold color, so the warning survives the category colors.
	L.WriteString(sep + ctxBarBy(barPct, ctxSegments(cm)) + " " + pctColor(barPct) + fmt.Sprintf("%d%%", barPct) + reset +
		dim + "/" + reset + dimGray + barLabel + reset)
	if cs.off {
		L.WriteString(" " + dimGray + "compact:off" + reset)
	}

	L.WriteString(sep + cyan + dir + reset)
	if g.branch != "" {
		L.WriteString(dim + ":" + reset + green + g.branch + red + g.dirty + reset)
	}
	// worktree.name is only set inside a Claude worktree session; the
	// workspace field covers any linked worktree you happen to be in.
	wt := ""
	if data.Worktree != nil {
		wt = data.Worktree.Name
	}
	if wt == "" && data.Workspace != nil {
		wt = data.Workspace.GitWorktree
	}
	// A worktree named after its branch or directory says nothing the
	// dir:branch segment didn't.
	if wt != "" && wt != g.branch && wt != dir {
		L.WriteString(sep + magenta + "⌥ " + wt + reset)
	}
	if dur != "" {
		L.WriteString(sep + white + dur + reset)
	}
	if costUsd > 0 {
		costStr := fmt.Sprintf("%.2f", costUsd)
		if costStr != "0.00" {
			L.WriteString(sep + white + "$" + costStr + reset)
		}
	}
	if data.Cost != nil && (data.Cost.LinesAdded > 0 || data.Cost.LinesRemoved > 0) {
		L.WriteString(sep + green + fmt.Sprintf("+%d", data.Cost.LinesAdded) + reset +
			" " + red + fmt.Sprintf("-%d", data.Cost.LinesRemoved) + reset)
	}
	if data.OutputStyle != nil && data.OutputStyle.Name != "" && data.OutputStyle.Name != "default" {
		L.WriteString(sep + dimGray + "style:" + reset + cyan + data.OutputStyle.Name + reset)
	}
	if data.Vim != nil && data.Vim.Mode != "" {
		L.WriteString(sep + yellow + data.Vim.Mode + reset)
	}
	if data.PR != nil && data.PR.Number > 0 {
		col := yellow
		switch data.PR.ReviewState {
		case "approved":
			col = green
		case "changes_requested":
			col = red
		case "draft":
			col = dimGray
		}
		kind := "PR"
		if data.PR.Kind == "mr" {
			kind = "MR"
		}
		L.WriteString(sep + col + link(fmt.Sprintf("%s #%d", kind, data.PR.Number), data.PR.URL) + reset)
	}
	if data.Agent != nil && data.Agent.Name != "" {
		L.WriteString(sep + magenta + "@" + data.Agent.Name + reset)
	}
	// Last on the line, where the eye lands after the session's own state.
	if n := updateNotice(updateTag); n != "" {
		L.WriteString(sep + n)
	}

	// ── Under line 1: the key to the bar's category colors, from the mod ──
	M := paintCtxKey(cm)

	// ── Line 2: this session's tokens — cache, in/out, the day's trend ────
	var C strings.Builder
	// Rendered whether or not the OAuth endpoint answers: it reads the
	// session's own transcript, which every account has.
	if s := paintTokens(tk, data.PromptCache, time.Now(), idleAt); s != "" {
		C.WriteString(s)
	}
	// The day's burn against the week's average: the same question as the
	// session totals just before it, over a longer span.
	if trend != "" {
		if C.Len() > 0 {
			C.WriteString(sep)
		}
		C.WriteString(trend)
	}

	// ── Line 3: plan quotas — 5h, 7d, spend, per-model, extra credits ─────
	var R strings.Builder

	// A reopened session gets no rate_limits until its first API response —
	// fall back to the last-known buckets so the line isn't blank until then,
	// dimmed to admit the numbers aren't live.
	rl, stale := data.RateLimits, false
	if hasBuckets(rl) {
		saveLimits(rl)
	} else if cached := loadLimits(time.Now()); cached != nil {
		rl, stale = cached, true
	}
	if hasBuckets(rl) {
		if rl.FiveHour != nil {
			writeBucket(&R, "5h", int(rl.FiveHour.UsedPercentage), rl.FiveHour.ResetsAt, stale)
		}
		if rl.SevenDay != nil {
			writeBucket(&R, "7d", int(rl.SevenDay.UsedPercentage), rl.SevenDay.ResetsAt, stale)
		}
		if rl.SpendLimit != nil {
			writeBucket(&R, "spend", int(rl.SpendLimit.UsedPercentage), rl.SpendLimit.ResetsAt, stale)
		}
	}

	// Per-model quotas (e.g. Fable) and extra credits come from the OAuth
	// usage endpoint — the statusline payload only carries 5h/7d.
	if u != nil {
		for _, s := range u.Scoped {
			writeBucket(&R, s.Name, s.Pct, s.ResetsAt, false)
		}
		// Dollars close the line — the one budget not counted in percent.
		if e := u.Extra; e != nil {
			if R.Len() > 0 {
				R.WriteString(sep)
			}
			R.WriteString(white + "extra" + reset + " " + white + "$" + e.Used + reset +
				dim + "/" + reset + dimGray + "$" + e.Limit + reset)
		}
	}

	// ── Line 4: the user's plugin, when one is set ────────────────────────
	T := pluginOut

	w := os.Stdout
	_, _ = io.WriteString(w, L.String()+"\n")
	for _, line := range []string{M, C.String(), R.String()} {
		if line != "" {
			_, _ = io.WriteString(w, line+"\n")
		}
	}
	if T != "" {
		_, _ = io.WriteString(w, T+"\n")
	}
}

// writeBucket renders one quota segment — "5h 34% ↻ 2h 30m" — for the
// payload's 5h/7d buckets and the per-model quotas alike. The percentage
// carries the severity; the reset time stays quiet beside them. A
// stale segment is last-known state restored across a restart, so it's drawn
// flat gray rather than in colors that imply a live reading.
func writeBucket(b *strings.Builder, name string, pct int, resetsAt int64, stale bool) {
	label, value := white, pctColor(pct)
	if stale {
		label, value = dimGray, dimGray
	}
	if b.Len() > 0 {
		b.WriteString(sep)
	}
	b.WriteString(label + name + reset + " " + value + fmt.Sprintf("%d%%", pct) + reset)
	if t := fmtRemaining(resetsAt); t != "" {
		b.WriteString(" " + dim + "↻" + reset + " " + dimGray + t + reset)
	}
}

// shortModelName drops a "(1M context)" style suffix — the context bar's
// "/1M" label already says it.
func shortModelName(name string) string {
	if i := strings.LastIndex(name, " ("); i > 0 && strings.HasSuffix(name, "context)") {
		return name[:i]
	}
	return name
}
