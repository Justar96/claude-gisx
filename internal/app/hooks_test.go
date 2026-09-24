package app

import (
	"os"
	"strings"
	"testing"
)

// answers stubs the terminal with canned replies, in order; ok=false once
// they run out, like a session with no terminal.
func answers(t *testing.T, replies ...string) *[]string {
	t.Helper()
	var asked []string
	old := ask
	ask = func(q string, hidden bool) (string, bool) {
		asked = append(asked, q)
		if len(replies) == 0 {
			return "", false
		}
		r := replies[0]
		replies = replies[1:]
		return r, true
	}
	t.Cleanup(func() { ask = old })
	return &asked
}

func clearKeys(t *testing.T) {
	for _, k := range []string{"TYPESAFE_API_KEY", "DEEPSEEK_API_KEY", promptHookOptOut} {
		t.Setenv(k, "")
	}
}

func TestPromptHookAddRemoveKeepsOtherHooks(t *testing.T) {
	theirs := map[string]any{"hooks": []any{map[string]any{"type": "command", "command": "my-linter"}}}
	s := map[string]any{"hooks": map[string]any{
		"UserPromptSubmit": []any{theirs},
		"Stop":             []any{map[string]any{}},
	}}
	addPromptHook(s)
	addPromptHook(s) // idempotent
	if n := len(promptHookEntries(s)); n != 2 || !hasPromptHook(s) {
		t.Fatalf("expected theirs + ours, got %d", n)
	}
	if !removePromptHook(s) || hasPromptHook(s) {
		t.Fatal("remove failed")
	}
	if n := len(promptHookEntries(s)); n != 1 {
		t.Fatalf("their hook was touched: %d left", n)
	}
	if _, ok := s["hooks"].(map[string]any)["Stop"]; !ok {
		t.Fatal("other events must be kept")
	}
}

func TestRemovePromptHookCleansEmptyContainers(t *testing.T) {
	s := map[string]any{}
	addPromptHook(s)
	removePromptHook(s)
	if _, ok := s["hooks"]; ok {
		t.Fatalf("left %v behind", s)
	}
}

func TestOfferPromptHookYesAndNo(t *testing.T) {
	isolateHome(t)
	clearKeys(t)
	answers(t, "y")
	s := map[string]any{}
	if !offerPromptHook(s, hookAsk) || !hasPromptHook(s) {
		t.Fatal("a yes should install")
	}

	s = map[string]any{}
	asked := answers(t, "n")
	if offerPromptHook(s, hookAsk) || hasPromptHook(s) {
		t.Fatal("a no should not install")
	}
	// Remembered: the next setup or update doesn't ask again.
	if offerPromptHook(s, hookAsk) || len(*asked) != 1 {
		t.Fatalf("asked %d times after a no", len(*asked))
	}
	if promptHookNudge() != "" {
		t.Fatal("no nudge after a no")
	}
	// --hook overrides the earlier no and clears it.
	if !offerPromptHook(s, hookYes) || isOff(loadConfig("").value(promptHookOptOut)) {
		t.Fatal("--hook should install and forget the no")
	}
}

func TestOfferPromptHookWithoutTerminal(t *testing.T) {
	isolateHome(t)
	clearKeys(t)
	answers(t) // no terminal
	s := map[string]any{}
	if offerPromptHook(s, hookAsk) || hasPromptHook(s) {
		t.Fatal("nothing to install without an answer")
	}
	if isOff(loadConfig("").value(promptHookOptOut)) {
		t.Fatal("no terminal is not a no")
	}
	if promptHookNudge() == "" {
		t.Fatal("undecided users should see the nudge")
	}
}

func TestOfferAPIKeys(t *testing.T) {
	isolateHome(t)
	clearKeys(t)
	asked := answers(t, "ts-secret", "")
	offerAPIKeys("", true, false)
	if len(*asked) != 2 {
		t.Fatalf("expected both keys asked, got %d", len(*asked))
	}
	cfg := loadConfig("")
	if cfg.value("TYPESAFE_API_KEY") != "ts-secret" || cfg.value("DEEPSEEK_API_KEY") != "" {
		t.Fatalf("got ts=%q ds=%q", cfg.value("TYPESAFE_API_KEY"), cfg.value("DEEPSEEK_API_KEY"))
	}
	if st, err := os.Stat(gisxConfigPath()); err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("config.json holds secrets and must be 0600: %v %v", st.Mode(), err)
	}
	// A key set anywhere isn't asked for again; DeepSeek is skipped without the hook.
	asked = answers(t)
	offerAPIKeys("", false, false)
	if len(*asked) != 0 {
		t.Fatalf("asked %q", *asked)
	}
}

func TestSetGisxConfigKeepsOtherKeys(t *testing.T) {
	isolateHome(t)
	clearKeys(t)
	writeFile(t, gisxConfigPath(), `{"CLAUDE_GISX_SUGGEST_MIN":0.4}`)
	if err := setGisxConfig("TYPESAFE_API_KEY", "k"); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(gisxConfigPath())
	if !strings.Contains(string(raw), "SUGGEST_MIN") || !strings.Contains(string(raw), `"k"`) {
		t.Fatalf("got %s", raw)
	}
}

func TestOfferAPIKeysPromotesProjectKeys(t *testing.T) {
	isolateHome(t)
	clearKeys(t)
	project := t.TempDir()
	writeFile(t, project+"/.env", "TYPESAFE_API_KEY=ts-proj\nDEEPSEEK_API_KEY=ds-proj\n")
	// Enter (default yes) for TypeSafe, "n" for DeepSeek.
	answers(t, "", "n")
	offerAPIKeys(project, true, false)
	global := loadConfig("")
	if global.value("TYPESAFE_API_KEY") != "ts-proj" {
		t.Fatal("accepted key should be saved for every project")
	}
	if global.value("DEEPSEEK_API_KEY") != "" {
		t.Fatal("declined key must stay project-only")
	}
	// Now global: setup doesn't ask about it again.
	asked := answers(t, "n")
	offerAPIKeys(project, true, false)
	if len(*asked) != 1 {
		t.Fatalf("expected only the DeepSeek question, got %q", *asked)
	}
}

func TestKeysCommandAsksEvenWhenSet(t *testing.T) {
	isolateHome(t)
	clearKeys(t)
	writeFile(t, gisxConfigPath(), `{"TYPESAFE_API_KEY":"old"}`)
	asked := answers(t, "", "ds-new") // keep TypeSafe, set DeepSeek
	offerAPIKeys("", false, true)
	if len(*asked) != 2 {
		t.Fatalf("keys should ask for both, got %d", len(*asked))
	}
	cfg := loadConfig("")
	if cfg.value("TYPESAFE_API_KEY") != "old" || cfg.value("DEEPSEEK_API_KEY") != "ds-new" {
		t.Fatalf("got ts=%q ds=%q", cfg.value("TYPESAFE_API_KEY"), cfg.value("DEEPSEEK_API_KEY"))
	}
}
