package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
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
		L.WriteString(dim + "/" + reset + effortColor(effortLevel) + effortLevel + reset)
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
	if data.RateLimits != nil {
		if data.RateLimits.FiveHour != nil {
			fivePct = int(data.RateLimits.FiveHour.UsedPercentage)
		}
		if data.RateLimits.SevenDay != nil {
			weekPct = int(data.RateLimits.SevenDay.UsedPercentage)
		}
		fiveRem := 100 - fivePct
		weekRem := 100 - weekPct
		R.WriteString(white + "5h" + reset + " " + pctColor(fivePct) + fmt.Sprintf("%d%%", fivePct) + reset)
		if data.RateLimits.FiveHour != nil {
			if s := fmtRemaining(data.RateLimits.FiveHour.ResetsAt); s != "" {
				R.WriteString(" " + dimGray + "resets" + reset + " " + remainingColor(fiveRem) + s + reset)
			}
		}
		R.WriteString(sep + white + "7d" + reset + " " + pctColor(weekPct) + fmt.Sprintf("%d%%", weekPct) + reset)
		if data.RateLimits.SevenDay != nil {
			if s := fmtRemaining(data.RateLimits.SevenDay.ResetsAt); s != "" {
				R.WriteString(" " + dimGray + "resets" + reset + " " + remainingColor(weekRem) + s + reset)
			}
		}
	}
	limitPct = maxi(fivePct, weekPct)

	// Per-model quotas (e.g. Fable) and extra credits come from the OAuth
	// usage endpoint — the statusline payload only carries 5h/7d.
	if u := fetchUsage(context.Background()); u != nil {
		for _, s := range u.Scoped {
			if R.Len() > 0 {
				R.WriteString(sep)
			}
			R.WriteString(white + s.Name + reset + " " + pctColor(s.Pct) + fmt.Sprintf("%d%%", s.Pct) + reset)
			if t := fmtRemaining(s.ResetsAt); t != "" {
				R.WriteString(" " + dimGray + "resets" + reset + " " + remainingColor(100-s.Pct) + t + reset)
			}
			limitPct = maxi(limitPct, s.Pct)
		}
		if u.Extra != nil {
			if R.Len() > 0 {
				R.WriteString(sep)
			}
			R.WriteString(dimGray + "extra" + reset + " " + white + "$" + u.Extra.Used +
				dimGray + "/" + reset + white + "$" + u.Extra.Limit + reset)
		}
	}
	if trend := weeklyTrend(); trend != "" {
		if R.Len() > 0 {
			R.WriteString(sep)
		}
		R.WriteString(trend)
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
