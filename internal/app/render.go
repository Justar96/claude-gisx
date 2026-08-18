package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type sessionInput struct {
	Model         *modelInfo     `json:"model,omitempty"`
	Workspace     *workspaceInfo `json:"workspace,omitempty"`
	Cwd           string         `json:"cwd,omitempty"`
	ContextWindow *contextInfo   `json:"context_window,omitempty"`
	Exceeds200k   bool           `json:"exceeds_200k_tokens,omitempty"`
	Cost          *costInfo      `json:"cost,omitempty"`
	Effort        *levelInfo     `json:"effort,omitempty"`
	FastMode      bool           `json:"fast_mode,omitempty"`
	OutputStyle   *nameInfo      `json:"output_style,omitempty"`
	Vim           *modeInfo      `json:"vim,omitempty"`
	Agent         *nameInfo      `json:"agent,omitempty"`
	PR            *prInfo        `json:"pr,omitempty"`
	Worktree      *nameInfo      `json:"worktree,omitempty"`
	RateLimits    *rateLimits    `json:"rate_limits,omitempty"`
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
}
type contextInfo struct {
	UsedPercentage    *float64 `json:"used_percentage,omitempty"`
	ContextWindowSize int      `json:"context_window_size,omitempty"`
}
type costInfo struct {
	TotalCostUSD    float64 `json:"total_cost_usd,omitempty"`
	TotalDurationMS int64   `json:"total_duration_ms,omitempty"`
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
	ReviewState string `json:"review_state,omitempty"`
}
type rateLimits struct {
	FiveHour *rateBucket `json:"five_hour,omitempty"`
	SevenDay *rateBucket `json:"seven_day,omitempty"`
}
type rateBucket struct {
	UsedPercentage float64 `json:"used_percentage,omitempty"`
	ResetsAt       int64   `json:"resets_at,omitempty"`
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

	// Runs here rather than off cacheDir(): a dev build with no plugin
	// configured never reaches that path, and the strays would linger.
	cleanLegacyCache()

	modelName := "Claude"
	if data.Model != nil && data.Model.DisplayName != "" {
		modelName = data.Model.DisplayName
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
	has1M := ctxSize > 200_000

	// `effort` is only sent for models that support it — absent means the
	// model has no effort dial, so don't invent one from settings.
	effortLevel := ""
	if data.Effort != nil {
		effortLevel = data.Effort.Level
	}

	cs := computeCompactState(ctxSize)
	g := gitInfo(cwd)
	dir := filepath.Base(cwd)
	dur := fmtDuration(durationMs)

	// ── Line 1: model + bar + workspace + session meta ────────────────────
	var L strings.Builder
	L.WriteString(bold + blue + modelName + reset)
	if effortLevel != "" {
		L.WriteString(" " + dimGray + "effort" + reset + " " + effortColor(effortLevel) + effortLevel + reset)
	}
	if data.FastMode {
		// A word, not an emoji — emoji widths vary per terminal and shift the line.
		L.WriteString(sep + yellow + "fast" + reset)
	}
	L.WriteString(sep + ctxBar(pctUsed) + " " + pctColor(pctUsed) + fmt.Sprintf("%d%%", pctUsed) + reset +
		dim + "/" + reset + dimGray + fmtCtxLabel(ctxSize) + reset)

	if cs.userSet && cs.pct > 0 && pctUsed >= cs.pct {
		L.WriteString(" " + bold + red + fmt.Sprintf("⚠ compact %d%%", cs.pct) + reset)
	}
	if cs.off {
		L.WriteString(" " + dimGray + "compact:off" + reset)
	}
	if has1M && data.Exceeds200k {
		L.WriteString(" " + dimGray + "+ext" + reset)
	}
	// Sits with the context bar rather than on the usage line: both are
	// "how much am I burning", and line 2 only renders when there are quotas
	// to report, which a fresh session doesn't have yet.
	if trend := tokenTrend(); trend != "" {
		L.WriteString(sep + trend)
	}

	L.WriteString(sep + cyan + dir + reset)
	if g.branch != "" {
		L.WriteString(dim + ":" + reset + green + g.branch + red + g.dirty + reset)
	}
	if data.Worktree != nil && data.Worktree.Name != "" {
		L.WriteString(sep + magenta + "⌥ " + data.Worktree.Name + reset)
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
		L.WriteString(sep + col + fmt.Sprintf("PR #%d", data.PR.Number) + reset)
	}
	if data.Agent != nil && data.Agent.Name != "" {
		L.WriteString(sep + magenta + "@" + data.Agent.Name + reset)
	}

	// ── Line 2: rate limits + extra credits ───────────────────────────────
	var R strings.Builder
	fivePct := 0
	weekPct := 0
	limitPct := 0

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
			fivePct = int(rl.FiveHour.UsedPercentage)
			writeBucket(&R, "5h", fivePct, rl.FiveHour.ResetsAt, stale)
		}
		if rl.SevenDay != nil {
			weekPct = int(rl.SevenDay.UsedPercentage)
			writeBucket(&R, "7d", weekPct, rl.SevenDay.ResetsAt, stale)
		}
	}
	limitPct = maxi(fivePct, weekPct)

	// Per-model quotas (e.g. Fable) and extra credits come from the OAuth
	// usage endpoint — the statusline payload only carries 5h/7d.
	u := fetchUsage(context.Background())
	if u != nil {
		for _, s := range u.Scoped {
			writeBucket(&R, s.Name, s.Pct, s.ResetsAt, false)
			limitPct = maxi(limitPct, s.Pct)
		}
	}

	// Sits with the quotas, not after the credits: it's the same kind of
	// reading on the same scale — how much of a budget is gone — asked of the
	// session's own input instead of the plan's. Rendered outside the OAuth
	// block because it holds for accounts that endpoint says nothing about.
	if tk := paintTokens(sessionTokens(data.Transcript)); tk != "" {
		if R.Len() > 0 {
			R.WriteString(sep)
		}
		R.WriteString(tk)
	}

	// Dollars close the line — the one figure here that isn't a percentage.
	if u != nil && u.Extra != nil {
		if R.Len() > 0 {
			R.WriteString(sep)
		}
		R.WriteString(dimGray + "extra" + reset + " " + white + "$" + u.Extra.Used +
			dimGray + "/" + reset + white + "$" + u.Extra.Limit + reset)
	}

	// ── Line 3: plugin output or built-in tip ─────────────────────────────
	pluginOut := runPlugin(stdinJSON)
	T := tipLine(tipContext{
		ctxPct:   pctUsed,
		limitPct: limitPct,
		weekPct:  weekPct,
		compact:  cs,
		plugin:   pluginOut,
	})

	w := os.Stdout
	_, _ = io.WriteString(w, L.String()+"\n")
	if R.Len() > 0 {
		_, _ = io.WriteString(w, R.String()+"\n")
	}
	if T != "" {
		_, _ = io.WriteString(w, T+"\n")
	}
}

// writeBucket renders one quota segment — "5h 12% resets 2h 30m" — for the
// payload's 5h/7d buckets and the per-model quotas alike. A stale segment is
// last-known state restored across a restart, so it's drawn flat gray rather
// than in the severity colors that imply a live reading.
func writeBucket(b *strings.Builder, name string, pct int, resetsAt int64, stale bool) {
	label, value, left := white, pctColor(pct), remainingColor(100-pct)
	if stale {
		label, value, left = dimGray, dimGray, dimGray
	}
	if b.Len() > 0 {
		b.WriteString(sep)
	}
	b.WriteString(label + name + reset + " " + value + fmt.Sprintf("%d%%", pct) + reset)
	if t := fmtRemaining(resetsAt); t != "" {
		b.WriteString(" " + dimGray + "resets" + reset + " " + left + t + reset)
	}
}
