package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unicode/utf16"
)

// `claude-gisx hook prompt` is a UserPromptSubmit hook that restates a prompt
// with clearer structure and grammar. Claude Code doesn't let a hook replace
// the prompt text, so there are two ways to use the rewrite:
//
//   - auto: when TypeSafe rates a prompt hard to read, DeepSeek rewrites it,
//     TypeSafe checks the rewrite still asks for the same thing, and the
//     result goes to Claude as additionalContext next to the original.
//   - trigger: a prompt starting with "??" is blocked instead of sent, and the
//     rewrite is shown and copied to the clipboard for you to paste and send.
//
// Every failure lets the prompt through untouched: a hook that errors or
// hangs stalls the session, which is worse than an unpolished prompt.

type promptHookInput struct {
	Prompt         string `json:"prompt"`
	Cwd            string `json:"cwd"`
	TranscriptPath string `json:"transcript_path"`
}

type promptHookOutput struct {
	Decision               string           `json:"decision,omitempty"`
	Reason                 string           `json:"reason,omitempty"`
	SuppressOriginalPrompt bool             `json:"suppressOriginalPrompt,omitempty"`
	HookSpecificOutput     *hookSpecificOut `json:"hookSpecificOutput,omitempty"`
}

type hookSpecificOut struct {
	HookEventName     string `json:"hookEventName"`
	AdditionalContext string `json:"additionalContext,omitempty"`
}

const (
	defaultRewriteTrigger = "??"
	defaultRewriteModel   = "deepseek-flash"
	defaultRewriteTimeout = 8 * time.Second
	defaultRewriteBelow   = 1.5 // clarity score (0–2) under which auto mode rewrites
	rewriteKeepNoul       = 0.7 // "same meaning" probability a rewrite must reach
	maxRewriteRunes       = 4000
)

const rewriteSystemPrompt = `You rewrite a developer's message to Claude Code, an AI coding agent, so it is clear and well structured.
Fix grammar and spelling, put the asks in a logical order, and use a short numbered list when there are several distinct asks.
Keep the meaning exactly: do not add requirements, drop details, answer the message, or change code, file paths, commands, identifiers, numbers, or quoted text.
Keep the user's language and first-person voice, and keep it about as long as the original.
Reply with the rewritten message only: no preamble, no quotes, no code fence around it.`

func promptHookCmd(stdin io.Reader, stdout io.Writer) int {
	var in promptHookInput
	if err := json.NewDecoder(io.LimitReader(stdin, 4<<20)).Decode(&in); err != nil {
		return 0
	}
	cfg := loadConfig(in.Cwd)
	out := handlePrompt(context.Background(), cfg, in)
	if out != nil {
		_ = json.NewEncoder(stdout).Encode(out)
	}
	return 0
}

// handlePrompt returns the hook's JSON, or nil to let the prompt through as is.
func handlePrompt(ctx context.Context, cfg gisxConfig, in promptHookInput) *promptHookOutput {
	dsKey := cfg.value("DEEPSEEK_API_KEY")
	if dsKey == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, rewriteTimeout(cfg))
	defer cancel()

	trigger := cfg.value("CLAUDE_GISX_REWRITE_TRIGGER")
	if trigger == "" {
		trigger = defaultRewriteTrigger
	}
	if rest, ok := strings.CutPrefix(strings.TrimSpace(in.Prompt), trigger); ok {
		return triggeredRewrite(ctx, cfg, dsKey, strings.TrimSpace(rest))
	}

	// Anything below ends with either a fresh rewrite on record or none, so
	// the statusline never shows a rewrite of an earlier prompt.
	clearRewrite(in.TranscriptPath)
	if isOff(cfg.value("CLAUDE_GISX_REWRITE_AUTO")) || !worthRewriting(in.Prompt) {
		return nil
	}
	tsKey := cfg.value("TYPESAFE_API_KEY")
	if tsKey != "" {
		clarity, err := promptClarity(ctx, cfg, tsKey, in.Prompt)
		if err != nil || clarity >= parseScore(cfg.value("CLAUDE_GISX_REWRITE_BELOW"), defaultRewriteBelow) {
			return nil
		}
	}
	rewritten, err := deepseekRewrite(ctx, cfg, dsKey, in.Prompt)
	if err != nil || rewritten == "" || rewritten == strings.TrimSpace(in.Prompt) {
		return nil
	}
	if tsKey != "" {
		same, err := sameMeaning(ctx, cfg, tsKey, in.Prompt, rewritten)
		if err != nil || same < rewriteKeepNoul {
			return nil
		}
	}
	saveRewrite(in.TranscriptPath, rewritten)
	return &promptHookOutput{HookSpecificOutput: &hookSpecificOut{
		HookEventName: "UserPromptSubmit",
		AdditionalContext: "claude-gisx restated the user's prompt with clearer structure and grammar. " +
			"The user's original wording is authoritative; use this restatement only to resolve ambiguity.\n\n" + rewritten,
	}}
}

// triggeredRewrite blocks the prompt and hands the rewrite back to the user.
// The block is deliberate either way: sending "?? fix it" to Claude as is
// would be the one outcome nobody asked for.
func triggeredRewrite(ctx context.Context, cfg gisxConfig, dsKey, text string) *promptHookOutput {
	if text == "" {
		return &promptHookOutput{Decision: "block", Reason: "claude-gisx: nothing to rewrite after the trigger"}
	}
	rewritten, err := deepseekRewrite(ctx, cfg, dsKey, text)
	if err != nil || rewritten == "" {
		msg := "no text came back"
		if err != nil {
			msg = err.Error()
		}
		return &promptHookOutput{Decision: "block", Reason: "claude-gisx: rewrite failed (" + msg + "); your prompt was not sent"}
	}
	how := "paste it to send"
	if copyToClipboard(rewritten) {
		how = "copied to your clipboard, paste it to send"
	}
	return &promptHookOutput{
		Decision:               "block",
		Reason:                 "✦ rewritten, " + how + ":\n\n" + rewritten,
		SuppressOriginalPrompt: true,
	}
}

// worthRewriting skips what a rewrite would only damage or can't improve:
// slash commands and shell escapes, one-word replies, pasted content, code.
func worthRewriting(p string) bool {
	p = strings.TrimSpace(p)
	if p == "" || strings.ContainsAny(p[:1], "/!#") {
		return false
	}
	if len(strings.Fields(p)) < 4 || len([]rune(p)) > maxRewriteRunes {
		return false
	}
	return !strings.Contains(p, "<pasted_content") && !strings.Contains(p, "```")
}

func isOff(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "0", "off", "false", "no":
		return true
	}
	return false
}

func rewriteTimeout(cfg gisxConfig) time.Duration {
	var sec float64
	if _, err := fmt.Sscanf(cfg.value("CLAUDE_GISX_REWRITE_TIMEOUT"), "%g", &sec); err == nil && sec > 0 {
		return time.Duration(sec * float64(time.Second))
	}
	return defaultRewriteTimeout
}

func parseScore(s string, def float64) float64 {
	var f float64
	if _, err := fmt.Sscanf(s, "%g", &f); err != nil || f < 0 || f > 2 {
		return def
	}
	return f
}

// ── TypeSafe judgments ────────────────────────────────────────────────────

type scoreResponse struct {
	Answers map[string]struct {
		Score float64 `json:"score"`
		Noul  float64 `json:"noul"`
	} `json:"answers"`
}

// promptClarity scores how easy the prompt is to act on, 0 (hard) to 2
// (clear). Only a low score is worth the latency of a rewrite.
func promptClarity(ctx context.Context, cfg gisxConfig, apiKey, prompt string) (float64, error) {
	body := map[string]any{
		"model": suggestModel(cfg),
		"state": map[string]any{"prompt": prompt},
		"questions": map[string]any{
			"clarity": map[string]any{
				"type":         "score",
				"instructions": "`prompt` is a developer's message to an AI coding agent. How easy is it to understand exactly what is being asked?",
				"criteria": []string{
					"Hard to act on: garbled grammar, unclear about what to do, or several asks run together with no structure.",
					"Understandable with effort: the intent comes through, but grammar, word order, or structure make it easy to misread.",
					"Clear: an engineer reads it once and knows exactly what is asked; a typo or two at most.",
				},
			},
		},
	}
	var r scoreResponse
	if err := postJSON(ctx, suggestEndpoint(cfg), apiKey, body, &r, rewriteTimeout(cfg)); err != nil {
		return 0, err
	}
	a, ok := r.Answers["clarity"]
	if !ok {
		return 0, fmt.Errorf("no clarity answer")
	}
	return a.Score, nil
}

// sameMeaning is the probability the rewrite asks for what the original did.
func sameMeaning(ctx context.Context, cfg gisxConfig, apiKey, original, rewritten string) (float64, error) {
	body := map[string]any{
		"model": suggestModel(cfg),
		"state": map[string]any{"original": original, "rewritten": rewritten},
		"questions": map[string]any{
			"same": map[string]any{
				"type": "noul",
				"instructions": "Does `rewritten` ask for exactly the same thing as `original`: the same tasks, constraints, " +
					"file names, identifiers, and values, with no requirement added, dropped, or changed?",
			},
		},
	}
	var r scoreResponse
	if err := postJSON(ctx, suggestEndpoint(cfg), apiKey, body, &r, rewriteTimeout(cfg)); err != nil {
		return 0, err
	}
	a, ok := r.Answers["same"]
	if !ok {
		return 0, fmt.Errorf("no same answer")
	}
	return a.Noul, nil
}

// ── DeepSeek rewrite ──────────────────────────────────────────────────────

func deepseekEndpoint(cfg gisxConfig) string {
	base := strings.TrimRight(cfg.value("DEEPSEEK_BASE_URL"), "/")
	if base == "" {
		base = "https://api.deepseek.com"
	}
	return base + "/chat/completions"
}

func deepseekRewrite(ctx context.Context, cfg gisxConfig, apiKey, prompt string) (string, error) {
	model := cfg.value("CLAUDE_GISX_REWRITE_MODEL")
	if model == "" {
		model = defaultRewriteModel
	}
	body := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": rewriteSystemPrompt},
			{"role": "user", "content": prompt},
		},
		// A copy edit needs no reasoning, and thinking would eat the budget.
		"thinking":    map[string]string{"type": "disabled"},
		"temperature": 0.2,
		"max_tokens":  1500,
		"stream":      false,
	}
	var r struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := postJSON(ctx, deepseekEndpoint(cfg), apiKey, body, &r, rewriteTimeout(cfg)); err != nil {
		return "", err
	}
	if len(r.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}
	return cleanRewrite(r.Choices[0].Message.Content), nil
}

// cleanRewrite strips the wrapping a model adds despite being told not to.
func cleanRewrite(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") && strings.HasSuffix(s, "```") && len(s) >= 6 {
		s = strings.TrimSpace(s[3 : len(s)-3])
		if i := strings.IndexByte(s, '\n'); i >= 0 && !strings.Contains(s[:i], " ") {
			s = strings.TrimSpace(s[i+1:]) // a language tag on the fence line
		}
	}
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		s = strings.TrimSpace(s[1 : len(s)-1])
	}
	return s
}

// ── clipboard ─────────────────────────────────────────────────────────────

// copyToClipboard tries the platform's clipboard tools in turn. clip.exe
// (Windows, and WSL through interop) reads UTF-16 with a BOM correctly but
// mangles raw UTF-8, so it gets the text transcoded.
func copyToClipboard(text string) bool {
	type tool struct {
		name  string
		args  []string
		utf16 bool
	}
	var tools []tool
	switch runtime.GOOS {
	case "windows":
		tools = []tool{{"clip", nil, true}}
	case "darwin":
		tools = []tool{{"pbcopy", nil, false}}
	default:
		tools = []tool{
			{"clip.exe", nil, true},
			{"wl-copy", nil, false},
			{"xclip", []string{"-selection", "clipboard"}, false},
			{"xsel", []string{"--clipboard", "--input"}, false},
		}
	}
	for _, t := range tools {
		path, err := exec.LookPath(t.name)
		if err != nil {
			continue
		}
		data := []byte(text)
		if t.utf16 {
			data = utf16LE(text)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		cmd := exec.CommandContext(ctx, path, t.args...)
		cmd.Stdin = bytes.NewReader(data)
		err = cmd.Run()
		cancel()
		if err == nil {
			return true
		}
	}
	return false
}

func utf16LE(s string) []byte {
	units := utf16.Encode([]rune(s))
	out := make([]byte, 2, 2+2*len(units))
	out[0], out[1] = 0xFF, 0xFE
	for _, u := range units {
		out = append(out, byte(u), byte(u>>8))
	}
	return out
}

// ── statusline hand-off ───────────────────────────────────────────────────

// The hook and the statusline are separate processes; this file is how the
// third line learns what Claude was given for the current prompt.
type rewriteRecord struct {
	Text string `json:"text"`
	At   int64  `json:"at"`
}

func rewriteCachePath(transcriptPath string) string {
	sid := strings.TrimSuffix(filepath.Base(transcriptPath), ".jsonl")
	return filepath.Join(lineCacheDir(), "statusline-rewrite-"+sid+".json")
}

func saveRewrite(transcriptPath, text string) {
	if transcriptPath == "" {
		return
	}
	if raw, err := json.Marshal(rewriteRecord{Text: text, At: time.Now().Unix()}); err == nil {
		_ = writeFileAtomic(rewriteCachePath(transcriptPath), raw)
	}
}

func clearRewrite(transcriptPath string) {
	if transcriptPath != "" {
		_ = os.Remove(rewriteCachePath(transcriptPath))
	}
}

// A rewrite is news for the turn it was made for; past this it's stale.
const rewriteShowFor = 30 * time.Minute

func lastRewrite(transcriptPath string) string {
	if transcriptPath == "" {
		return ""
	}
	raw, err := os.ReadFile(rewriteCachePath(transcriptPath))
	if err != nil {
		return ""
	}
	var r rewriteRecord
	if json.Unmarshal(raw, &r) != nil || time.Since(time.Unix(r.At, 0)) > rewriteShowFor {
		return ""
	}
	return r.Text
}

// paintRewrite renders "rewrote ❯ Add a --dry-run flag to…" on one line.
func paintRewrite(text string) string {
	text = strings.Join(strings.Fields(text), " ")
	if text == "" {
		return ""
	}
	return dimGray + "rewrote" + reset + " " + dim + "❯" + reset + " " + white + truncate(text, 100) + reset
}
