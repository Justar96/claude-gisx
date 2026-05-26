package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

type extraUsage struct {
	Used  string
	Limit string
}

type oauthBody struct {
	ExtraUsage *struct {
		IsEnabled    bool    `json:"is_enabled"`
		UsedCredits  float64 `json:"used_credits"`
		MonthlyLimit float64 `json:"monthly_limit"`
	} `json:"extra_usage,omitempty"`
}

func fetchExtraUsage(ctx context.Context) *extraUsage {
	cachePath := filepath.Join(cacheDir(), "statusline-extra-cache.json")
	body := readCachedOAuth(cachePath, 60*time.Second)
	if body == nil {
		body = doOAuthFetch(ctx, cachePath)
	}
	if body == nil {
		// last-resort: any cached body, regardless of age
		body = readCachedOAuth(cachePath, 365*24*time.Hour)
	}
	if body == nil || body.ExtraUsage == nil || !body.ExtraUsage.IsEnabled {
		return nil
	}
	return &extraUsage{
		Used:  fmt.Sprintf("%.2f", body.ExtraUsage.UsedCredits/100),
		Limit: fmt.Sprintf("%.2f", body.ExtraUsage.MonthlyLimit/100),
	}
}

func readCachedOAuth(p string, maxAge time.Duration) *oauthBody {
	st, err := os.Stat(p)
	if err != nil {
		return nil
	}
	if time.Since(st.ModTime()) > maxAge {
		return nil
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		return nil
	}
	var b oauthBody
	if err := json.Unmarshal(raw, &b); err != nil {
		return nil
	}
	return &b
}

func doOAuthFetch(ctx context.Context, cachePath string) *oauthBody {
	token := getOAuthToken()
	if token == "" {
		return nil
	}
	reqCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, "GET", "https://api.anthropic.com/api/oauth/usage", nil)
	if err != nil {
		return nil
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("anthropic-beta", "oauth-2025-04-20")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil
	}
	var b oauthBody
	if err := json.Unmarshal(raw, &b); err != nil {
		return nil
	}
	if b.ExtraUsage == nil {
		return nil
	}
	_ = os.MkdirAll(filepath.Dir(cachePath), 0o755)
	_ = os.WriteFile(cachePath, raw, 0o644)
	return &b
}

func getOAuthToken() string {
	if t := os.Getenv("CLAUDE_CODE_OAUTH_TOKEN"); t != "" {
		return t
	}
	home, err := os.UserHomeDir()
	if err == nil {
		raw, err := os.ReadFile(filepath.Join(home, ".claude", ".credentials.json"))
		if err == nil {
			var c struct {
				ClaudeAiOauth struct {
					AccessToken string `json:"accessToken"`
				} `json:"claudeAiOauth"`
			}
			if json.Unmarshal(raw, &c) == nil && c.ClaudeAiOauth.AccessToken != "" {
				return c.ClaudeAiOauth.AccessToken
			}
		}
	}
	// secret-tool (libsecret) fallback — Linux only
	cmdCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(cmdCtx, "secret-tool", "lookup", "service", "Claude Code-credentials").Output()
	if err != nil {
		return ""
	}
	var c struct {
		ClaudeAiOauth struct {
			AccessToken string `json:"accessToken"`
		} `json:"claudeAiOauth"`
	}
	if json.Unmarshal(out, &c) == nil {
		return c.ClaudeAiOauth.AccessToken
	}
	return ""
}

func cacheDir() string {
	return filepath.Join(os.TempDir(), "claude")
}
