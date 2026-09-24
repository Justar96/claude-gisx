package app

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestConfigPrecedence(t *testing.T) {
	isolateHome(t)
	t.Setenv("TYPESAFE_API_KEY", "")
	project := t.TempDir()
	projLocal := filepath.Join(project, ".claude", "settings.local.json")
	projShared := filepath.Join(project, ".claude", "settings.json")

	writeFile(t, gisxConfigPath(), `{"TYPESAFE_API_KEY":"gisx","CLAUDE_GISX_SUGGEST_MIN":0.4}`)
	if v, src := loadConfig(project).get("TYPESAFE_API_KEY"); v != "gisx" || src != gisxConfigPath() {
		t.Fatalf("gisx config: got %q from %q", v, src)
	}
	if v := loadConfig(project).value("CLAUDE_GISX_SUGGEST_MIN"); v != "0.4" {
		t.Fatalf("numeric value: got %q", v)
	}

	writeFile(t, settingsPath(), `{"env":{"TYPESAFE_API_KEY":"user"}}`)
	if v := loadConfig(project).value("TYPESAFE_API_KEY"); v != "user" {
		t.Fatalf("user settings should beat gisx config, got %q", v)
	}

	writeFile(t, projShared, `{"env":{"TYPESAFE_API_KEY":"shared"}}`)
	if v := loadConfig(project).value("TYPESAFE_API_KEY"); v != "shared" {
		t.Fatalf("project settings should beat user settings, got %q", v)
	}

	writeFile(t, projLocal, `{"env":{"TYPESAFE_API_KEY":"local"}}`)
	if v, src := loadConfig(project).get("TYPESAFE_API_KEY"); v != "local" || src != projLocal {
		t.Fatalf("settings.local.json should win among files, got %q from %q", v, src)
	}

	t.Setenv("TYPESAFE_API_KEY", "env")
	if v, src := loadConfig(project).get("TYPESAFE_API_KEY"); v != "env" || src != "env" {
		t.Fatalf("environment should win, got %q from %q", v, src)
	}
}

func TestConfigIgnoresBrokenFiles(t *testing.T) {
	isolateHome(t)
	t.Setenv("TYPESAFE_API_KEY", "")
	writeFile(t, settingsPath(), `{not json`)
	writeFile(t, gisxConfigPath(), `{"TYPESAFE_API_KEY":"gisx"}`)
	if v := loadConfig("").value("TYPESAFE_API_KEY"); v != "gisx" {
		t.Fatalf("a broken settings file should be skipped, got %q", v)
	}
}

func TestNextPromptReadsKeyFromConfig(t *testing.T) {
	isolateHome(t)
	t.Setenv("TYPESAFE_API_KEY", "")
	t.Setenv("CLAUDE_GISX_NO_SUGGEST", "")
	project := t.TempDir()
	writeFile(t, filepath.Join(project, ".claude", "settings.local.json"),
		`{"env":{"TYPESAFE_API_KEY":"k","CLAUDE_GISX_NO_SUGGEST":"1"}}`)
	transcript := filepath.Join(t.TempDir(), "sess.jsonl")
	writeFile(t, transcript, finishedTurn)
	// The key is found, and the opt-out beside it is honored too — so no call.
	if got := nextPrompt(transcript, project, workspaceFacts{}); got != "" {
		t.Fatalf("got %q", got)
	}
	if _, err := os.Stat(suggestCachePath(transcript)); err == nil {
		t.Fatal("opted out, but a request was attempted")
	}
}

func TestDotenv(t *testing.T) {
	isolateHome(t)
	t.Setenv("TYPESAFE_API_KEY", "")
	t.Setenv("DEEPSEEK_API_KEY", "")
	project := t.TempDir()
	writeFile(t, filepath.Join(project, ".env"), "# keys\nexport TYPESAFE_API_KEY=\"ts-1\"\r\nDEEPSEEK_API_KEY=ds-2 # comment\n\nBROKEN\n")
	cfg := loadConfig(project)
	if v, src := cfg.get("TYPESAFE_API_KEY"); v != "ts-1" || filepath.Base(src) != ".env" {
		t.Fatalf("got %q from %q", v, src)
	}
	if v := cfg.value("DEEPSEEK_API_KEY"); v != "ds-2" {
		t.Fatalf("got %q", v)
	}
	// settings.local.json still outranks .env.
	writeFile(t, filepath.Join(project, ".claude", "settings.local.json"), `{"env":{"TYPESAFE_API_KEY":"local"}}`)
	if v := loadConfig(project).value("TYPESAFE_API_KEY"); v != "local" {
		t.Fatalf("got %q", v)
	}
}
