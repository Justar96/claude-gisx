# claude-gisx

A rich, dynamic statusline for [Claude Code](https://docs.claude.com/en/docs/claude-code) — single self-contained binary, no runtime deps.

```
Opus 4.8/high · ████░░░░░░░░░░░ 28%/1M · myproject:main · 42m · $1.20 · +128 -31 · PR #42
5h 32% resets 3h 12m · 7d 8% resets 5d 2h · Fable 61% resets 5d 2h · cache 96% · in 1.5M · out 15.1k · tokens 15.7M ▼47% · extra $4.20/$20.00
next ❯ run the tests
```

## Install

### Linux / macOS

```bash
curl -fsSL https://raw.githubusercontent.com/Justar96/claude-gisx/main/install.sh | bash
```

### Windows (PowerShell)

```powershell
irm https://raw.githubusercontent.com/Justar96/claude-gisx/main/install.ps1 | iex
```

The installer:

1. Detects your OS + arch and downloads the matching binary from [GitHub Releases](https://github.com/Justar96/claude-gisx/releases/latest).
2. Verifies the SHA-256 checksum if `SHA256SUMS` is published with the release.
3. Drops the binary at `~/.local/bin/claude-gisx` (Linux/macOS) or `%LOCALAPPDATA%\Programs\claude-gisx\claude-gisx.exe` (Windows).
4. Runs `claude-gisx setup` to wire it into `~/.claude/settings.json`. Your existing `statusLine` is backed up first. Setup also sets `hideVimModeIndicator` (left alone if you already set it), since the statusline renders the vim mode itself.
5. Asks `Install the prompt-rewrite hook? [y/N]` and, for any key it can't already find, asks you to paste your TypeSafe and DeepSeek API keys. Input is hidden, and Enter skips. Keys are saved to `~/.claude/.gisx/config.json`, readable only by you. A "no" is remembered, so updates won't ask again. With no terminal (CI, scripts), it skips both questions and prints how to do it later.

#### Installer options

| Env var | Description |
|---------|-------------|
| `CLAUDE_GISX_VERSION` | Pin a release tag (e.g. `v1.0.0`). Default: `latest`. |
| `CLAUDE_GISX_INSTALL_DIR` | Override install dir. Default: `~/.local/bin` (Unix) / `%LOCALAPPDATA%\Programs\claude-gisx` (Windows). |
| `CLAUDE_GISX_REPO` | Pull binaries from a fork: `owner/repo`. |
| `CLAUDE_GISX_SKIP_SETUP` | Install the binary only, don't touch settings.json. |
| `CLAUDE_GISX_HOOK` | `yes` or `no` answers the prompt-hook question in advance, e.g. for unattended installs. |

Restart Claude Code once done.

## CLI

The same `claude-gisx` binary does everything. Run it with no args for an at-a-glance setup screen:

```
$ claude-gisx

  claude-gisx v1.0.0
  rich, dynamic statusline for Claude Code

  ✓ installed — restart Claude Code if you just installed

  Get started
  • claude-gisx setup       wire into ~/.claude/settings.json
  • claude-gisx status      show current install state and backup
  • claude-gisx uninstall   restore your previous statusLine
  • claude-gisx help        this screen

  …
```

| Command | What it does |
|---------|--------------|
| `claude-gisx` _(no stdin, no args)_ | Show the install/setup help screen. |
| `claude-gisx setup` | Write `~/.claude/settings.json` so Claude Code uses claude-gisx. Backs up any existing `statusLine` to `~/.claude/.gisx/`. |
| `claude-gisx status` | Inspect current settings and backup state. |
| `claude-gisx update` | Download the latest release, verify its SHA-256 against the published `SHA256SUMS`, and replace the running binary in place. `--check` only reports, `--force` reinstalls the current version. |
| `claude-gisx uninstall` | Restore the previous `statusLine` (or remove it), and remove the prompt hook. |
| `claude-gisx hook install` | Offer the prompt-rewrite hook and ask for any missing API keys. `--yes` installs without asking. `setup --hook` / `--no-hook` do the same during setup. `update` runs this with the newly installed binary. |
| `claude-gisx hook uninstall` | Remove the prompt-rewrite hook and leave your other hooks alone. |
| `claude-gisx help` / `--help` | Help screen. |
| `claude-gisx version` | Print the binary version. |
| `<json> \| claude-gisx` | Render a statusline from session JSON on stdin. This is what Claude Code calls. |

## Features

- **Model-aware context bar** — single `pct%/Nk` reading sized to the live `context_window.context_window_size` (so a 200k Haiku shows `42%/200k` and a 1M Opus shows `28%/1M`). Clean, mark-free bar that color-shifts as usage grows. Since the label carries the window size, a `(1M context)` suffix on the model name is dropped
- **Auto-compact notice** — reads Claude Code's actual auto-compact env vars (`CLAUDE_AUTOCOMPACT_PCT_OVERRIDE`, `CLAUDE_CODE_AUTO_COMPACT_WINDOW`, `DISABLE_COMPACT`). When you've configured a threshold, a bold red `⚠ compact N%` badge appears once usage reaches it. With `DISABLE_COMPACT` set, the badge becomes a dim `compact:off`. No env var set → no badge
- **Dynamic color coding** — green / cyan / orange / yellow / red based on usage thresholds
- **Rate limit tracking** — 5-hour and 7-day usage with time-to-reset, plus any per-model quota your plan tracks separately (e.g. `Fable 61% resets 5d 2h`). Behind a Claude apps gateway, the `spend` limit from `rate_limits.spend_limit` joins them (it can pass 100%)
- **Daily token trend** — `tokens 15.7M ▼47%` follows the session totals on the usage line: the newest day of `dailyModelTokens` in `~/.claude/stats-cache.json`, against the daily average of the 7 days before it (green `▲` up, dim `▼` down). Measured against that cache's own newest date and hidden once it's >10 days stale, since Claude Code only refreshes it when you open `/usage`
- **Cache hit rate and session token split** — `cache 96% · in 1.5M · out 15.1k` sits with the quotas on the usage line: how much of this session's input the prompt cache absorbed, and the totals it's a share of. The rate is Claude Code's own `prompt_cache.hit_ratio` (v2.1.251+, the same figure `/usage` shows), falling back to a sum over the transcript on older versions; `cold` appears when the cached prefix has passed its TTL and the next request rebuilds it, and `N miss` counts prefixes invalidated with no compaction to explain it. `in`/`out` come from the session transcript's per-request `usage`, the only place the input/output split exists — `in` counts everything billed as input (cached prefix included, once per request), `out` is completions. The percentage is graded on its own scale (green ≥90, cyan ≥75, yellow ≥50, red below) because a healthy session lives in the 90s and the quota thresholds would paint that whole band one flat green. Read incrementally from where the last render stopped, so a multi-megabyte transcript costs a seek, not a re-parse
- **Git integration** — branch name and dirty state (one `git status --no-optional-locks`, so it never fights a commit in progress), plus worktree (`⌥`, for Claude worktree sessions and any linked worktree via `workspace.git_worktree`; hidden when it's named after the directory or branch already shown) and PR/MR (`PR #1234` or `MR #12` for GitLab, colored by review state, clickable via OSC 8 where the terminal supports it) indicators when present
- **Session name** — the `--name`/`/rename` name or the AI-generated title, truncated to 32 characters, so parallel sessions read apart at a glance
- **Session duration**, **cost**, and **lines changed** (`+128 -31`) from the live `cost.*` fields
- **Effort level** rendered right on the model name (`Claude Opus 4.8/high`), from the session's `effort.level` — hidden for models with no effort dial
- **Fast mode** chip (`fast`) when the session has fast mode on
- **Extra usage** credits display (OAuth accounts)
- **Vim mode**, **output style**, and **subagent** indicators when active
- **Next-prompt suggestion** — with `TYPESAFE_API_KEY` set, the third line suggests what to send next once a turn ends: `next ❯ run the tests`. See [Next-prompt suggestion](#next-prompt-suggestion)
- **Prompt rewrite hook** — `claude-gisx hook prompt` restates unclear prompts with DeepSeek before Claude reads them, or on demand with a `??` prefix. See [Prompt rewrite](#prompt-rewrite)
- **Rotating third line** — the fallback when there's no suggestion: **what's new** parsed from Claude Code's own changelog cache (`~/.claude/cache/changelog.md`, feature entries only — fixes and links are skipped), plus **usage nudges**: `weekly 65% used · /usage-credits raise your weekly limit` past 50%. All local, no network. Warnings (auto-compact, rate limit) take the line 2 renders out of 3 — they stay true for hours, so they yield every third render instead of blocking the rotation outright
- **Update notice** — when a newer release exists, the third line says so in a drifting red-to-pink tint: `✦ claude-gisx v1.2.2 available · claude-gisx update`. GitHub is checked at most once every 6 hours (cached in the temp dir, 1.5s timeout, silent on failure). Dev builds never nag; set `CLAUDE_GISX_NO_UPDATE_CHECK=1` to disable entirely
- **Third-line plugin** — point `CLAUDE_GISX_PLUGIN` at any shell command (including `curl` against your own API) and its stdout becomes the third line. Timeout + caching are built in. See [Third-line plugin](#third-line-plugin)

## Manual setup

If you'd rather skip the installer wiring, add this yourself to `~/.claude/settings.json`:

```json
{
  "statusLine": {
    "type": "command",
    "command": "claude-gisx",
    "hideVimModeIndicator": true
  }
}
```

The `command` must resolve on the `PATH` Claude Code uses; if `~/.local/bin` isn't on it, use the absolute path.

**Windows**: use forward slashes in the path (`"C:/Users/you/AppData/Local/Programs/claude-gisx/claude-gisx.exe"`). Claude Code routes the command through Git Bash on Windows, which interprets backslashes as escape characters and silently drops stdout. The installer writes forward slashes automatically; this only matters if you edit settings.json by hand.

## Uninstall

```bash
claude-gisx uninstall
rm ~/.local/bin/claude-gisx       # or %LOCALAPPDATA%\Programs\claude-gisx on Windows
```

`uninstall` restores whatever `statusLine` was there before claude-gisx (or removes it if you had none).

## Context bar legend

```
████░░░░░░░░░░░ 28%/1M
```

The percentage and bar are sized to the live `context_window.context_window_size` from the session JSON, so a Haiku 200k session shows `42%/200k` while an Opus 1M session shows `28%/1M`.

| Usage | Color |
|-------|-------|
| < 50% | Green / Cyan |
| 50%+ | Orange |
| 70%+ | Yellow |
| 90%+ | Red |

## Auto-compact notice

Claude Code's auto-compact threshold is controlled by environment variables (it is **not** exposed in `settings.json`). The `⚠ compact N%` badge mirrors those env vars and is **opt-in** — it appears only when you've configured one of them.

| Env var | Effect |
|---------|--------|
| `CLAUDE_AUTOCOMPACT_PCT_OVERRIDE` | Set the % of the context window at which auto-compact triggers (1–100). Claude Code's built-in default is ~95% when unset. |
| `CLAUDE_CODE_AUTO_COMPACT_WINDOW` | Treat a smaller token window as "full" for compaction. The badge remaps the threshold onto the displayed `used_percentage` scale so the % shown matches reality. |
| `DISABLE_COMPACT` | Disable auto-compact entirely. The badge is suppressed and a dim `compact:off` appears next to the bar. |
| `CLAUDE_GISX_COMPACT_PCT` | gisx-only override for the badge (does **not** change Claude Code's actual behavior). |

```bash
# Trigger compact at 80% of the model's full context.
CLAUDE_AUTOCOMPACT_PCT_OVERRIDE=80 claude

# On a 1M model, treat 500k as the working window. With the ~95% default,
# the badge fires when used_percentage ≥ ~47% (= 95% × 500000 / 1000000).
CLAUDE_CODE_AUTO_COMPACT_WINDOW=500000 claude

# Disable auto-compact; statusline shows "compact:off".
DISABLE_COMPACT=1 claude
```

## Next-prompt suggestion

When a turn ends, the third line suggests the prompt that most likely moves the work forward:

```
next ❯ run the tests
```

It uses [TypeSafe](https://docs.typesafe.ai/introduction)'s Jev model, which returns a typed pick instead of generated text. One request per finished turn sends your last prompt, the start and end of Claude's reply (2,000 characters each, at most), and a few workspace facts: branch, uncommitted changes, lines changed, open PR, and context used. The request asks two questions:

- a **Choice** over a fixed catalog: `run the tests`, `fix the failing tests`, `continue`, `/code-review`, `add tests for this`, `commit these changes`, `open a PR`, `update the README`, `explain what you changed`, `/security-review`, `/compact`, or `none`
- a **Noul**: does the reply end by asking you something? If it does, nothing is suggested, because the next move is your answer.

The pick shows only when it isn't `none` and its probability clears the threshold. It's cached against the turn's final transcript entry, so later redraws don't call the API again. Nothing is suggested while a turn is still running. If the request fails, is slow, or no key is set, the line falls back to the rotation below.

| Env var | Default | Description |
|---------|---------|-------------|
| `TYPESAFE_API_KEY` | _(unset)_ | Turns the feature on. Get a key at [console.typesafe.ai](https://console.typesafe.ai/keys). |
| `CLAUDE_GISX_SUGGEST_MIN` | `0.30` | Minimum probability the top pick needs before it's shown (0–1). |
| `CLAUDE_GISX_NO_SUGGEST` | _(unset)_ | Set to anything to turn suggestions off while keeping the key. |
| `TYPESAFE_BASE_URL` | `https://api.typesafe.ai` | API base URL. |
| `TYPESAFE_DEFAULT_MODEL` | `jev-latest` | Model to use. Pin a version such as `jev-1.13.0` if you've tuned the threshold against it. |

### Where to put the key

Each of these settings can be an environment variable or an entry in a config file. The statusline reads the files itself, so a change takes effect on the next redraw with no restart. First match wins:

1. the environment
2. `<project>/.claude/settings.local.json`, under `env`. Claude Code gitignores this file, so it's the right place for a per-project key.
3. `<project>/.env`, as `KEY=value` lines. Make sure it's gitignored.
4. `<project>/.claude/settings.json`, under `env`. This file is usually committed, so don't put a key here.
5. `~/.claude/settings.json`, under `env`
6. `~/.claude/.gisx/config.json`, as flat keys. Use this to set a key once for every project.

```json
// ~/.claude/settings.json or <project>/.claude/settings.local.json
{ "env": { "TYPESAFE_API_KEY": "ts-..." } }
```

```json
// ~/.claude/.gisx/config.json
{ "TYPESAFE_API_KEY": "ts-...", "CLAUDE_GISX_SUGGEST_MIN": 0.4 }
```

`claude-gisx status`, run from a project, shows whether suggestions are on and which file the key came from. `uninstall` leaves `config.json` in place.

## Prompt rewrite

`claude-gisx hook prompt` is a `UserPromptSubmit` hook that restates a prompt with clearer structure and grammar, using DeepSeek. Claude Code doesn't let a hook replace the prompt you typed, so the rewrite is used in one of two ways.

**Auto.** This runs on every prompt:
1. TypeSafe scores how clear the prompt is.
2. Only a prompt that scores low goes to DeepSeek for a rewrite.
3. TypeSafe checks that the rewrite still asks for the same thing. If it doesn't, the rewrite is dropped.
4. Claude gets the rewrite as extra context next to your original, which stays authoritative.

The statusline's third line shows `rewrote ❯ …` while the turn runs, so you can see what Claude was given. Slash commands, `!` shell escapes, replies shorter than four words, pasted content, and prompts containing code blocks are never rewritten. Without a TypeSafe key, every other prompt is rewritten and nothing is checked.

**Trigger.** A prompt that starts with `??` isn't sent. The hook shows the rewrite instead and copies it to your clipboard (`clip.exe` on Windows and WSL, `pbcopy`, `wl-copy`, `xclip`, or `xsel`). Paste it to send it.

```
> ?? fix bug where statusline it show wrong branch when worktree
  ✦ rewritten, copied to your clipboard, paste it to send:

  Fix the bug where the statusline shows the wrong branch when using a worktree.
```

If anything fails or times out, the prompt goes through unchanged. The one exception is a failed `??` rewrite: that prompt is blocked with the error, since sending it as typed isn't what you asked for.

Privacy: the prompts it rewrites are sent to DeepSeek, and with a TypeSafe key every prompt eligible for auto mode is also sent to TypeSafe for scoring.

`claude-gisx setup` and `claude-gisx hook install` add it for you. To add it by hand, put this in `~/.claude/settings.json`:

```json
{
  "hooks": {
    "UserPromptSubmit": [
      { "hooks": [ { "type": "command", "command": "claude-gisx hook prompt", "timeout": 15 } ] }
    ]
  }
}
```

| Setting | Default | Description |
|---------|---------|-------------|
| `DEEPSEEK_API_KEY` | _(unset)_ | Required; the hook does nothing without it. It's read from the same places as the other keys, including `<project>/.env`. |
| `CLAUDE_GISX_REWRITE_AUTO` | on | Set to `off` to keep only the `??` trigger. |
| `CLAUDE_GISX_REWRITE_TRIGGER` | `??` | Prefix that asks for a rewrite you review yourself. |
| `CLAUDE_GISX_REWRITE_BELOW` | `1.5` | Auto mode rewrites prompts with a clarity score below this. The scale runs from 0 (hard to act on) to 2 (clear). |
| `CLAUDE_GISX_REWRITE_TIMEOUT` | `8` | Seconds allowed for each API call. |
| `CLAUDE_GISX_REWRITE_MODEL` | `deepseek-flash` | DeepSeek model. |
| `DEEPSEEK_BASE_URL` | `https://api.deepseek.com` | API base URL. |

## Third-line plugin

The bottom (tip) line is a plugin point. Wire it to any shell command — including a `curl` call against your own API — and that command's stdout becomes the third line.

### Contract

| Env var | Default | Description |
|---------|---------|-------------|
| `CLAUDE_GISX_PLUGIN` | _(unset)_ | Shell command or script path. When set, replaces the rotating what's-new / usage line. |
| `CLAUDE_GISX_PLUGIN_TIMEOUT` | `2` | Seconds before the plugin is killed. Keep this low so the statusline stays responsive. |
| `CLAUDE_GISX_PLUGIN_CACHE` | `30` | Seconds to cache the plugin's last successful stdout. Prevents hammering your API. |
| `CLAUDE_GISX_NO_UPDATE_CHECK` | _(unset)_ | Set to anything to skip the GitHub release check and its update notice. |
| `CLAUDE_GISX_REPO` | `Justar96/claude-gisx` | Repo that `update` checks and downloads from. |

- The plugin receives the **full session JSON on stdin** (same shape the statusline gets). Use `jq` to pull fields like `session_id`, `model.id`, `cost.total_cost_usd`.
- First line of stdout becomes the third line (truncated to 500 chars). ANSI escapes (`\033[...`) are honored.
- On timeout / empty stdout the last cached output is reused. If there's no cache, the built-in tip line shows.
- Warnings always win — `⚠ compact` and `rate limit high` override the plugin. The plugin in turn outranks the next-prompt suggestion, the update notice, and the tip rotation.

### Connect to your own API

```bash
# ~/.claude/statusline-plugin.sh
#!/bin/bash
session_id=$(jq -r '.session_id // ""')
curl -s --max-time 1 \
    -H "Authorization: Bearer $MY_API_TOKEN" \
    "https://my.api.example.com/usage?session=$session_id" \
  | jq -r '"[36mtokens[0m " + (.tokens_remaining|tostring) +
           " [2m·[0m [33mbudget[0m $" + (.budget_remaining|tostring)'
```

```bash
chmod +x ~/.claude/statusline-plugin.sh
export CLAUDE_GISX_PLUGIN=~/.claude/statusline-plugin.sh
export CLAUDE_GISX_PLUGIN_CACHE=60   # poll your API at most once per minute
```

## Development

Requires Go 1.23+. Pure Go, no cgo, no external dependencies — `go build` produces a static binary in seconds.

```bash
git clone https://github.com/Justar96/claude-gisx
cd claude-gisx

make build         # build for host platform → ./claude-gisx
make test          # go test -race
make check         # fmt-check + vet + test (what CI runs)
make dist          # cross-compile all five release targets → dist/
make run           # build and render with a sample stdin fixture
```

Releases ship `linux-x64`, `linux-arm64`, `darwin-x64`, `darwin-arm64`, and `windows-x64` binaries (~6 MB each). Tag with `v*` to trigger `.github/workflows/release.yml`, which uploads the five binaries plus `SHA256SUMS` to GitHub Releases. Every push/PR runs `.github/workflows/ci.yml` (vet + gofmt + tests + smoke build).

## License

[MIT](LICENSE)
