package app

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// The third line suggests what to type next. Picking a follow-up is a small
// judgment — "given what Claude just said, which of these fits?" — so it goes
// to TypeSafe's System One model as a Choice over a fixed catalog rather than
// to a generative model: the answer is always one of the prompts below, never
// invented text, and it comes back with a probability we can gate on.
//
// Opt-in: nothing leaves the machine unless TYPESAFE_API_KEY is set, in the
// environment or a config file (see config.go).

// suggestion is one prompt the user might send next. `when` is the criteria
// text the model reads; the key is only for code.
type suggestion struct {
	key    string
	prompt string
	when   string
}

var suggestionCatalog = []suggestion{
	{"run_tests", "run the tests", "Claude changed code in this turn and has not run the tests or otherwise verified the change works."},
	{"fix_failures", "fix the failing tests", "The reply reports failing tests, a broken build, errors, or a bug that is still present."},
	{"continue", "continue", "The reply stopped partway through a multi-step task and names work that is still left to do."},
	{"code_review", "/code-review", "A non-trivial code change is finished and verified but has not been reviewed yet."},
	{"add_tests", "add tests for this", "New behavior was added or a bug was fixed without any tests covering it."},
	{"commit", "commit these changes", "The work is finished and verified, and the workspace has uncommitted changes."},
	{"open_pr", "open a PR", "The changes are committed on a feature branch (not main or master) and no pull request is open yet."},
	{"update_docs", "update the README", "User-visible behavior, flags, or configuration changed and the reply did not update the docs."},
	{"explain", "explain what you changed", "Claude made large or sweeping changes but the reply explains them only briefly."},
	{"security_review", "/security-review", "The change touches authentication, secrets, credentials, permissions, or parsing of untrusted input."},
	{"compact", "/compact", "The context window is mostly used and the task that was being worked on has just finished."},
	{"none", "", "Nothing above clearly fits, the reply is a plain answer to a question, or the user must reply with information only they have."},
}

const (
	suggestTimeout   = 2500 * time.Millisecond
	suggestRetry     = time.Minute // after a failed call, before trying the same turn again
	suggestTailBytes = 512 << 10
	suggestTextLimit = 2000 // characters of each message sent as state
	defaultSuggestP  = 0.30
	awaitingCutoff   = 0.5
)

func suggestEndpoint(cfg gisxConfig) string {
	base := strings.TrimRight(cfg.value("TYPESAFE_BASE_URL"), "/")
	if base == "" {
		base = "https://api.typesafe.ai"
	}
	return base + "/v1/systemone"
}

// turnTail is the finished exchange the suggestion is about.
type turnTail struct {
	key    string // uuid of the final assistant entry — changes once per turn
	prompt string // the user's last typed prompt
	reply  string // Claude's text since that prompt
}

type tailEntry struct {
	Type        string `json:"type"`
	UUID        string `json:"uuid"`
	IsMeta      bool   `json:"isMeta"`
	IsSidechain bool   `json:"isSidechain"`
	Message     struct {
		Role       string          `json:"role"`
		StopReason string          `json:"stop_reason"`
		Content    json.RawMessage `json:"content"`
	} `json:"message"`
}

// lastTurn reads the end of the transcript and returns the exchange that just
// finished, or ok=false while a turn is still running — a suggestion
// mid-turn would be about a reply that doesn't exist yet.
func lastTurn(transcriptPath string) (turnTail, bool) {
	f, err := os.Open(transcriptPath)
	if err != nil {
		return turnTail{}, false
	}
	defer f.Close()
	if st, err := f.Stat(); err == nil && st.Size() > suggestTailBytes {
		_, _ = f.Seek(st.Size()-suggestTailBytes, io.SeekStart)
	}
	return parseTurn(f)
}

func parseTurn(r io.Reader) (turnTail, bool) {
	var (
		t        turnTail
		reply    []string
		lastType string
		lastStop string
	)
	br := bufio.NewReaderSize(r, 64<<10)
	for {
		line, err := br.ReadBytes('\n')
		if len(line) > 0 {
			var e tailEntry
			if json.Unmarshal(line, &e) == nil && !e.IsSidechain && (e.Type == "user" || e.Type == "assistant") {
				switch e.Type {
				case "user":
					if text, ok := typedPrompt(e); ok {
						t.prompt, reply = text, nil
					}
					lastType = "user"
				case "assistant":
					reply = append(reply, contentText(e.Message.Content)...)
					lastType, lastStop, t.key = "assistant", e.Message.StopReason, e.UUID
				}
			}
		}
		if err != nil {
			break
		}
	}
	if lastType != "assistant" || lastStop != "end_turn" || t.key == "" || t.prompt == "" {
		return turnTail{}, false
	}
	t.reply = strings.TrimSpace(strings.Join(reply, "\n"))
	if t.reply == "" {
		return turnTail{}, false
	}
	return t, true
}

// typedPrompt reports whether a user entry is something the person typed, as
// opposed to a tool result, a meta note, or a slash command's echo.
func typedPrompt(e tailEntry) (string, bool) {
	if e.IsMeta {
		return "", false
	}
	var s string
	if json.Unmarshal(e.Message.Content, &s) == nil {
		s = strings.TrimSpace(s)
		return s, s != "" && !strings.HasPrefix(s, "<")
	}
	var blocks []struct {
		Type string `json:"type"`
	}
	if json.Unmarshal(e.Message.Content, &blocks) != nil {
		return "", false
	}
	for _, b := range blocks {
		if b.Type == "tool_result" {
			return "", false
		}
	}
	text := strings.TrimSpace(strings.Join(contentText(e.Message.Content), "\n"))
	return text, text != "" && !strings.HasPrefix(text, "<")
}

func contentText(raw json.RawMessage) []string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return []string{s}
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &blocks) != nil {
		return nil
	}
	var out []string
	for _, b := range blocks {
		if b.Type == "text" && strings.TrimSpace(b.Text) != "" {
			out = append(out, b.Text)
		}
	}
	return out
}

// workspaceFacts is what the model can't read off the conversation: whether
// there's anything to commit, which branch, how full the context is.
type workspaceFacts struct {
	Branch             string `json:"branch,omitempty"`
	UncommittedChanges bool   `json:"uncommitted_changes"`
	LinesAdded         int64  `json:"lines_added_this_session"`
	LinesRemoved       int64  `json:"lines_removed_this_session"`
	OpenPullRequest    bool   `json:"open_pull_request"`
	ContextUsedPercent int    `json:"context_window_used_percent"`
}

// headTail keeps the start and end of a long message — the opening says what
// was done, the ending says what's left — and drops the middle.
func headTail(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	h := n / 3
	return string(r[:h]) + "\n…\n" + string(r[len(r)-(n-h):])
}

func buildSuggestRequest(t turnTail, w workspaceFacts, model string) map[string]any {
	criteria := map[string]string{}
	for _, s := range suggestionCatalog {
		when := s.when
		if s.prompt != "" {
			when = fmt.Sprintf("Send %q. %s", s.prompt, s.when)
		}
		criteria[s.key] = when
	}
	return map[string]any{
		"model": model,
		"state": map[string]any{
			"last_user_prompt":     headTail(t.prompt, suggestTextLimit),
			"last_assistant_reply": headTail(t.reply, suggestTextLimit),
			"workspace":            w,
		},
		"questions": map[string]any{
			"next": map[string]any{
				"type": "choice",
				"instructions": "In a Claude Code coding session, the user sent `last_user_prompt` and Claude answered with " +
					"`last_assistant_reply`; `workspace` describes the repository right now. Which prompt should the user " +
					"most sensibly send next to move their work forward?",
				"criteria": criteria,
			},
			"awaiting": map[string]any{
				"type":         "noul",
				"instructions": "Does `last_assistant_reply` end by asking the user a question, or asking them to choose, confirm, or approve something before Claude continues?",
			},
		},
	}
}

func suggestModel(cfg gisxConfig) string {
	if m := cfg.value("TYPESAFE_DEFAULT_MODEL"); m != "" {
		return m
	}
	return "jev-latest"
}

type suggestResponse struct {
	Answers struct {
		Next struct {
			Choice        string             `json:"choice"`
			Probabilities map[string]float64 `json:"probabilities"`
		} `json:"next"`
		Awaiting struct {
			Noul float64 `json:"noul"`
		} `json:"awaiting"`
	} `json:"answers"`
}

// pickSuggestion applies the policy to the raw answers: stay quiet while
// Claude is waiting on the user, on "none", and when the top option is a
// weak favorite. Several prompts are often reasonable, so the gate is on the
// winner's own probability rather than on how concentrated the rest is.
func pickSuggestion(r suggestResponse, minP float64) string {
	if r.Answers.Awaiting.Noul >= awaitingCutoff {
		return ""
	}
	key := r.Answers.Next.Choice
	if key == "none" || r.Answers.Next.Probabilities[key] < minP {
		return ""
	}
	for _, s := range suggestionCatalog {
		if s.key == key {
			return key
		}
	}
	return ""
}

func callSuggest(ctx context.Context, endpoint, apiKey string, body map[string]any) (suggestResponse, error) {
	var out suggestResponse
	err := postJSON(ctx, endpoint, apiKey, body, &out, suggestTimeout)
	return out, err
}

// postJSON sends body with bearer auth and decodes a 200 response into out.
// Shared by the TypeSafe and DeepSeek calls, which differ only in shape.
func postJSON(ctx context.Context, url, apiKey string, body, out any, timeout time.Duration) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, "POST", url, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("%s returned %d", req.URL.Host, resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(out)
}

// Holds the chosen key, never painted text — see trendCache.
type suggestCache struct {
	Turn   string `json:"turn"`
	Key    string `json:"key"` // "" = nothing worth suggesting
	Failed bool   `json:"failed,omitempty"`
	At     int64  `json:"at"`
}

func suggestCachePath(transcriptPath string) string {
	sid := strings.TrimSuffix(filepath.Base(transcriptPath), ".jsonl")
	return filepath.Join(lineCacheDir(), "statusline-suggest-"+sid+".json")
}

// nextPrompt returns the catalog key to suggest for the turn that just ended,
// or "". One call per finished turn: the answer is cached against the final
// assistant entry, so the redraws that follow cost a file read.
func nextPrompt(transcriptPath, projectDir string, w workspaceFacts) string {
	if transcriptPath == "" {
		return ""
	}
	cfg := loadConfig(projectDir)
	apiKey := cfg.value("TYPESAFE_API_KEY")
	if apiKey == "" || cfg.value("CLAUDE_GISX_NO_SUGGEST") != "" {
		return ""
	}
	t, ok := lastTurn(transcriptPath)
	if !ok {
		return ""
	}
	path := suggestCachePath(transcriptPath)
	if raw, err := os.ReadFile(path); err == nil {
		var c suggestCache
		if json.Unmarshal(raw, &c) == nil && c.Turn == t.key &&
			(!c.Failed || time.Since(time.Unix(c.At, 0)) < suggestRetry) {
			return c.Key
		}
	}
	c := suggestCache{Turn: t.key, At: time.Now().Unix()}
	body := buildSuggestRequest(t, w, suggestModel(cfg))
	if r, err := callSuggest(context.Background(), suggestEndpoint(cfg), apiKey, body); err != nil {
		c.Failed = true
	} else {
		c.Key = pickSuggestion(r, parseProb(cfg.value("CLAUDE_GISX_SUGGEST_MIN"), defaultSuggestP))
	}
	if raw, err := json.Marshal(c); err == nil {
		_ = writeFileAtomic(path, raw)
	}
	return c.Key
}

func parseProb(s string, def float64) float64 {
	var f float64
	if _, err := fmt.Sscanf(s, "%g", &f); err != nil || f < 0 || f > 1 {
		return def
	}
	return f
}

// paintSuggestion renders "next ❯ run the tests".
func paintSuggestion(key string) string {
	for _, s := range suggestionCatalog {
		if s.key == key && s.prompt != "" {
			return dimGray + "next" + reset + " " + dim + "❯" + reset + " " + white + s.prompt + reset
		}
	}
	return ""
}
