package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

const finishedTurn = `{"type":"user","uuid":"u1","message":{"role":"user","content":"add a --dry-run flag"}}
{"type":"assistant","uuid":"a1","message":{"role":"assistant","stop_reason":"tool_use","content":[{"type":"tool_use"}]}}
{"type":"user","uuid":"u2","message":{"role":"user","content":[{"type":"tool_result"}]}}
{"type":"assistant","uuid":"a2","message":{"role":"assistant","stop_reason":"end_turn","content":[{"type":"thinking"}]}}
{"type":"assistant","uuid":"a3","message":{"role":"assistant","stop_reason":"end_turn","content":[{"type":"text","text":"Added --dry-run to main.go."}]}}
{"type":"last-prompt","lastPrompt":"x"}
`

func TestParseTurnFinished(t *testing.T) {
	tt, ok := parseTurn(strings.NewReader(finishedTurn))
	if !ok {
		t.Fatal("finished turn not recognised")
	}
	if tt.key != "a3" || tt.prompt != "add a --dry-run flag" || tt.reply != "Added --dry-run to main.go." {
		t.Fatalf("got %+v", tt)
	}
}

func TestParseTurnStillRunning(t *testing.T) {
	running := finishedTurn + `{"type":"user","uuid":"u3","message":{"role":"user","content":"now run it"}}` + "\n"
	if _, ok := parseTurn(strings.NewReader(running)); ok {
		t.Fatal("a turn waiting on Claude should not be suggested for")
	}
	mid := `{"type":"user","uuid":"u1","message":{"role":"user","content":"go"}}
{"type":"assistant","uuid":"a1","message":{"role":"assistant","stop_reason":"tool_use","content":[{"type":"text","text":"Looking."}]}}
`
	if _, ok := parseTurn(strings.NewReader(mid)); ok {
		t.Fatal("a tool_use stop is mid-turn")
	}
}

func TestParseTurnSkipsCommandEchoes(t *testing.T) {
	in := `{"type":"user","uuid":"u1","message":{"role":"user","content":"fix the bug"}}
{"type":"user","uuid":"u2","isMeta":true,"message":{"role":"user","content":"Caveat: local command"}}
{"type":"user","uuid":"u3","message":{"role":"user","content":"<command-name>/effort</command-name>"}}
{"type":"assistant","uuid":"a1","message":{"role":"assistant","stop_reason":"end_turn","content":[{"type":"text","text":"Fixed."}]}}
`
	tt, ok := parseTurn(strings.NewReader(in))
	if !ok || tt.prompt != "fix the bug" {
		t.Fatalf("got %+v, %v", tt, ok)
	}
}

func TestPickSuggestion(t *testing.T) {
	resp := func(choice string, p, awaiting float64) suggestResponse {
		var r suggestResponse
		r.Answers.Next.Choice = choice
		r.Answers.Next.Probabilities = map[string]float64{choice: p}
		r.Answers.Awaiting.Noul = awaiting
		return r
	}
	cases := []struct {
		name string
		r    suggestResponse
		want string
	}{
		{"clear winner", resp("run_tests", 0.7, 0.1), "run_tests"},
		{"weak winner", resp("run_tests", 0.2, 0.1), ""},
		{"none", resp("none", 0.9, 0.1), ""},
		{"awaiting the user", resp("commit", 0.9, 0.8), ""},
		{"unknown key", resp("rm_rf", 0.9, 0.1), ""},
	}
	for _, c := range cases {
		if got := pickSuggestion(c.r, defaultSuggestP); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestBuildSuggestRequestCoversCatalog(t *testing.T) {
	body := buildSuggestRequest(turnTail{prompt: "p", reply: "r"}, workspaceFacts{}, "jev-latest")
	next := body["questions"].(map[string]any)["next"].(map[string]any)
	criteria := next["criteria"].(map[string]string)
	if len(criteria) != len(suggestionCatalog) {
		t.Fatalf("criteria has %d options, catalog %d", len(criteria), len(suggestionCatalog))
	}
	if _, ok := criteria["none"]; !ok {
		t.Fatal("no-match option missing")
	}
}

func TestNextPromptCallsOncePerTurn(t *testing.T) {
	isolateHome(t)
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/v1/systemone" || r.Header.Get("Authorization") != "Bearer k" {
			w.WriteHeader(401)
			return
		}
		var body map[string]any
		if json.NewDecoder(r.Body).Decode(&body) != nil || body["model"] != "jev-latest" {
			w.WriteHeader(422)
			return
		}
		_, _ = w.Write([]byte(`{"model":"jev-1.13.0","answers":{
			"next":{"type":"choice","choice":"run_tests","probabilities":{"run_tests":0.8,"none":0.2},"confidence":0.7},
			"awaiting":{"type":"noul","noul":0.05}}}`))
	}))
	defer srv.Close()
	t.Setenv("TYPESAFE_API_KEY", "k")
	t.Setenv("TYPESAFE_BASE_URL", srv.URL)
	t.Setenv("TYPESAFE_DEFAULT_MODEL", "")
	t.Setenv("CLAUDE_GISX_NO_SUGGEST", "")

	transcript := filepath.Join(t.TempDir(), "sess.jsonl")
	if err := os.WriteFile(transcript, []byte(finishedTurn), 0o644); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if got := nextPrompt(transcript, "", workspaceFacts{}); got != "run_tests" {
			t.Fatalf("render %d: got %q", i, got)
		}
	}
	if n := calls.Load(); n != 1 {
		t.Fatalf("expected one API call for one turn, got %d", n)
	}
}

func TestNextPromptOffWithoutKey(t *testing.T) {
	isolateHome(t)
	t.Setenv("TYPESAFE_API_KEY", "")
	transcript := filepath.Join(t.TempDir(), "sess.jsonl")
	_ = os.WriteFile(transcript, []byte(finishedTurn), 0o644)
	if got := nextPrompt(transcript, "", workspaceFacts{}); got != "" {
		t.Fatalf("got %q with no key", got)
	}
}

func TestShortModelName(t *testing.T) {
	for in, want := range map[string]string{
		"Opus 5.5 (1M context)": "Opus 5.5",
		"Sonnet 5":              "Sonnet 5",
		"Claude (beta)":         "Claude (beta)",
	} {
		if got := shortModelName(in); got != want {
			t.Errorf("%q: got %q, want %q", in, got, want)
		}
	}
}
