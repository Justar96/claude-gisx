package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// fakeAPIs serves both TypeSafe and DeepSeek from one server, answering with
// the given clarity score and same-meaning probability.
type fakeAPIs struct {
	clarity, same float64
	rewrite       string
	dsCalls       atomic.Int32
	tsCalls       atomic.Int32
}

func (f *fakeAPIs) serve(t *testing.T) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		switch r.URL.Path {
		case "/chat/completions":
			f.dsCalls.Add(1)
			if r.Header.Get("Authorization") != "Bearer ds" || body["model"] != defaultRewriteModel {
				w.WriteHeader(401)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []any{map[string]any{"message": map[string]any{"content": f.rewrite}}},
			})
		case "/v1/systemone":
			f.tsCalls.Add(1)
			q := body["questions"].(map[string]any)
			answers := map[string]any{}
			if _, ok := q["clarity"]; ok {
				answers["clarity"] = map[string]any{"type": "score", "score": f.clarity}
			}
			if _, ok := q["same"]; ok {
				answers["same"] = map[string]any{"type": "noul", "noul": f.same}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"answers": answers})
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(srv.Close)
	t.Setenv("DEEPSEEK_API_KEY", "ds")
	t.Setenv("DEEPSEEK_BASE_URL", srv.URL)
	t.Setenv("TYPESAFE_API_KEY", "ts")
	t.Setenv("TYPESAFE_BASE_URL", srv.URL)
	for _, k := range []string{"TYPESAFE_DEFAULT_MODEL", "CLAUDE_GISX_REWRITE_AUTO", "CLAUDE_GISX_REWRITE_TRIGGER",
		"CLAUDE_GISX_REWRITE_MODEL", "CLAUDE_GISX_REWRITE_BELOW", "CLAUDE_GISX_REWRITE_TIMEOUT"} {
		t.Setenv(k, "")
	}
	return srv
}

func hook(t *testing.T, prompt, transcript string) *promptHookOutput {
	t.Helper()
	return handlePrompt(context.Background(), loadConfig(""), promptHookInput{Prompt: prompt, TranscriptPath: transcript})
}

const messyPrompt = "pls the flag dry run add it main.go and also test it maybe"

func TestAutoRewriteAddsContext(t *testing.T) {
	isolateHome(t)
	f := &fakeAPIs{clarity: 0.4, same: 0.95, rewrite: "Add a --dry-run flag to main.go, then test it."}
	f.serve(t)
	transcript := filepath.Join(t.TempDir(), "s.jsonl")

	out := hook(t, messyPrompt, transcript)
	if out == nil || out.HookSpecificOutput == nil || out.Decision != "" {
		t.Fatalf("expected additionalContext, got %+v", out)
	}
	if !strings.Contains(out.HookSpecificOutput.AdditionalContext, f.rewrite) {
		t.Fatalf("rewrite missing from context: %q", out.HookSpecificOutput.AdditionalContext)
	}
	if got := lastRewrite(transcript); got != f.rewrite {
		t.Fatalf("statusline hand-off: got %q", got)
	}
}

func TestAutoRewriteSkipsClearPrompts(t *testing.T) {
	isolateHome(t)
	f := &fakeAPIs{clarity: 1.9, same: 0.95, rewrite: "x"}
	f.serve(t)
	transcript := filepath.Join(t.TempDir(), "s.jsonl")
	saveRewrite(transcript, "an earlier prompt's rewrite")

	if out := hook(t, "Add a --dry-run flag to main.go and test it.", transcript); out != nil {
		t.Fatalf("clear prompt should pass through, got %+v", out)
	}
	if f.dsCalls.Load() != 0 {
		t.Fatal("DeepSeek called for a clear prompt")
	}
	if lastRewrite(transcript) != "" {
		t.Fatal("stale rewrite should be cleared on a new prompt")
	}
}

func TestAutoRewriteDropsChangedMeaning(t *testing.T) {
	isolateHome(t)
	f := &fakeAPIs{clarity: 0.4, same: 0.2, rewrite: "Delete main.go."}
	f.serve(t)
	if out := hook(t, messyPrompt, ""); out != nil {
		t.Fatalf("a rewrite that changed the ask must be dropped, got %+v", out)
	}
}

func TestAutoRewriteSkipsCommandsAndShortReplies(t *testing.T) {
	isolateHome(t)
	f := &fakeAPIs{clarity: 0, same: 1, rewrite: "x"}
	f.serve(t)
	for _, p := range []string{"/code-review high", "!ls -la", "yes do it", "fix this ```go\nfunc x() {}\n```"} {
		if out := hook(t, p, ""); out != nil {
			t.Errorf("%q: expected pass-through, got %+v", p, out)
		}
	}
	if f.tsCalls.Load()+f.dsCalls.Load() != 0 {
		t.Fatal("no API should be called for skipped prompts")
	}
}

func TestAutoRewriteCanBeTurnedOff(t *testing.T) {
	isolateHome(t)
	f := &fakeAPIs{clarity: 0, same: 1, rewrite: "x"}
	f.serve(t)
	t.Setenv("CLAUDE_GISX_REWRITE_AUTO", "off")
	if out := hook(t, messyPrompt, ""); out != nil {
		t.Fatalf("auto off, got %+v", out)
	}
}

func TestTriggerBlocksAndShowsRewrite(t *testing.T) {
	isolateHome(t)
	t.Setenv("PATH", "") // no clipboard tool: the rewrite must still be shown
	f := &fakeAPIs{clarity: 2, same: 1, rewrite: "Add a --dry-run flag to main.go."}
	f.serve(t)

	out := hook(t, "?? add dry run flag main.go", "")
	if out == nil || out.Decision != "block" || !out.SuppressOriginalPrompt {
		t.Fatalf("expected a block, got %+v", out)
	}
	if !strings.Contains(out.Reason, f.rewrite) || !strings.Contains(out.Reason, "paste it to send") {
		t.Fatalf("reason: %q", out.Reason)
	}
	if f.tsCalls.Load() != 0 {
		t.Fatal("the trigger skips the clarity gate")
	}
}

func TestTriggerFailureKeepsOriginalVisible(t *testing.T) {
	isolateHome(t)
	f := &fakeAPIs{}
	f.serve(t)
	t.Setenv("DEEPSEEK_API_KEY", "wrong")
	out := hook(t, "?? add dry run flag", "")
	if out == nil || out.Decision != "block" || out.SuppressOriginalPrompt || !strings.Contains(out.Reason, "not sent") {
		t.Fatalf("got %+v", out)
	}
}

func TestNoDeepSeekKeyPassesThrough(t *testing.T) {
	isolateHome(t)
	t.Setenv("DEEPSEEK_API_KEY", "")
	if out := hook(t, "?? anything at all here", ""); out != nil {
		t.Fatalf("got %+v", out)
	}
}

func TestPromptHookCmdWritesJSON(t *testing.T) {
	isolateHome(t)
	f := &fakeAPIs{clarity: 0.4, same: 0.95, rewrite: "Clean prompt."}
	f.serve(t)
	in := `{"prompt":"` + messyPrompt + `","cwd":"","transcript_path":""}`
	var out bytes.Buffer
	if code := promptHookCmd(strings.NewReader(in), &out); code != 0 {
		t.Fatalf("exit %d", code)
	}
	var got map[string]any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("not JSON: %q", out.String())
	}
	hso := got["hookSpecificOutput"].(map[string]any)
	if hso["hookEventName"] != "UserPromptSubmit" {
		t.Fatalf("got %v", got)
	}
}

func TestCleanRewrite(t *testing.T) {
	for in, want := range map[string]string{
		"  Add tests.  ":             "Add tests.",
		"\"Add tests.\"":             "Add tests.",
		"```\nAdd tests.\n```":       "Add tests.",
		"```text\nAdd tests.\n```":   "Add tests.",
		"Use `go test` then commit.": "Use `go test` then commit.",
	} {
		if got := cleanRewrite(in); got != want {
			t.Errorf("%q: got %q, want %q", in, got, want)
		}
	}
}

func TestUTF16LE(t *testing.T) {
	got := utf16LE("aก")
	want := []byte{0xFF, 0xFE, 'a', 0, 0x01, 0x0E}
	if !bytes.Equal(got, want) {
		t.Fatalf("got % x", got)
	}
}
