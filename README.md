# claude-gisx

A rich, dynamic statusline for [Claude Code](https://docs.claude.com/en/docs/claude-code) — single self-contained binary, no runtime deps.

```
Opus 4.8/high · ████████▌░ 86%/347k · myproject:main · 42m · $1.20 · +128 -31 · PR #42
ctx messages 224.8k · tools 28.3k · skills 10.0k · system 2.8k · free 100.0k · buffer 33.0k
cache warm 25m of 1h left · hit 96% · in 1.5M (1.4M read, 80.0k write, 2.0k uncached) · out 15.1k · daily 15.7M ▼47%
5h 32% ↻ 3h 12m · 7d 8% ↻ 5d 2h · Fable 61% ↻ 5d 2h · extra $4.20/$20.00
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
4. Runs `claude-gisx setup` to wire it into `~/.claude/settings.json`. Your existing `statusLine` is backed up first. Setup also sets `hideVimModeIndicator`, since the statusline renders the vim mode itself, and `refreshInterval: 30`, so the cache countdown keeps moving while you're idle instead of freezing at your last message. Either is left alone if you already set it. It also installs the [gisx mod](#the-gisx-mod) to `~/.claude/skills/gisx/`.

#### Installer options

| Env var | Description |
|---------|-------------|
| `CLAUDE_GISX_VERSION` | Pin a release tag (e.g. `v1.0.0`). Default: `latest`. |
| `CLAUDE_GISX_INSTALL_DIR` | Override install dir. Default: `~/.local/bin` (Unix) / `%LOCALAPPDATA%\Programs\claude-gisx` (Windows). |
| `CLAUDE_GISX_REPO` | Pull binaries from a fork: `owner/repo`. |
| `CLAUDE_GISX_SKIP_SETUP` | Install the binary only, don't touch settings.json. |

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
| `claude-gisx setup` | Write `~/.claude/settings.json` so Claude Code uses claude-gisx, and install the [gisx mod](#the-gisx-mod). Backs up any existing `statusLine` to `~/.claude/.gisx/`. On an install that's already active, it brings the settings and the mod up to date. |
| `claude-gisx status` | Inspect current settings and backup state. |
| `claude-gisx update` | Download the latest release, verify its SHA-256 against the published `SHA256SUMS`, and replace the running binary in place. `--check` only reports, `--force` reinstalls the current version. |
| `claude-gisx uninstall` | Restore the previous `statusLine` (or remove it), and remove the gisx mod. |
| `claude-gisx help` / `--help` | Help screen. |
| `claude-gisx version` | Print the binary version. |
| `<json> \| claude-gisx` | Render a statusline from session JSON on stdin. This is what Claude Code calls. |
| `claude-gisx ingest` | Store a context breakdown from stdin. The gisx mod calls it after each turn. |
| `claude-gisx sync` | Bring an active install's settings and mod up to date. `update` runs it on the new binary. |

## Features

- **Model-aware context bar** — single `pct%/Nk` reading sized to the live `context_window.context_window_size` (so a 200k Haiku shows `42%/200k` and a 1M Opus shows `28%/1M`). Clean, mark-free bar in one solid color that matches the percentage and changes as a whole when usage crosses a threshold. Ten cells wide, filled in eighth-cell steps, so it moves every 1.25% rather than every 10%. Since the label carries the window size, a `(1M context)` suffix on the model name is dropped
- **Auto-compact aware** — with an auto-compact window or percentage configured, the bar measures the runway to the point where Claude Code compacts (`86%/347k` = 86% of the way to compaction), read from the same env vars and settings Claude Code uses. See [Auto-compact](#auto-compact)
- **Dynamic color coding** — green / cyan / orange / yellow / red based on usage thresholds
- **Rate limit tracking** — line 3: 5-hour and 7-day usage with time-to-reset, plus any per-model quota your plan tracks separately (e.g. `Fable 61% ↻ 5d 2h`). Behind a Claude apps gateway, the `spend` limit from `rate_limits.spend_limit` joins them (it can pass 100%). Per-model quotas and extra credits come from the usage API, refreshed every 60s by a detached background process; renders show the cached figures and never wait on it
- **Daily token trend** — `daily 15.7M ▼47%` closes the session line (line 2): the newest day of `dailyModelTokens` in `~/.claude/stats-cache.json`, against the daily average of the 7 days before it (green `▲` up, dim `▼` down). Measured against that cache's own newest date and hidden once it's >10 days stale, since Claude Code only refreshes it when you open `/usage`
- **Prompt cache and session tokens** — line 2, in the terms of Anthropic's [prompt caching docs](https://platform.claude.com/docs/en/build-with-claude/prompt-caching): `cache warm 25m of 1h left · hit 96% 2 misses · in 1.5M (1.4M read, 80.0k write, 2.0k uncached) · out 15.1k`. **warm** means the cached prefix is still inside its TTL (`5m` or `1h`), with the time until it expires; **cold** means it has expired and the next request re-caches it (`recache 412.0k`); **off** means no response this session reported cache tokens. **hit** is cache reads as a share of all input, and **misses** count requests that re-processed content the cache already held, with no compaction to explain it. State, hit ratio and misses come from Claude Code's `prompt_cache` (v2.1.251+, the same figures `/usage` shows); older versions get the hit ratio from the transcript. **in** is total input = cache reads + cache writes + uncached input, summed from the transcript's per-request `usage` (`cache_read_input_tokens`, `cache_creation_input_tokens`, `input_tokens`), so it counts the cached prefix once per request, as billed. The hit ratio is graded on its own scale (green ≥90, cyan ≥75, yellow ≥50, red below), since a healthy session lives in the 90s. The transcript is read incrementally from where the last render stopped
- **Cache countdown and advice** — the time a warm cache has left is green while there's plenty, yellow from 40% of its lifetime down, and red in the last minute: any message before it lapses refreshes the entry at the read price. A cold cache whose prefix is 100k tokens or more adds `/compact first`, since the next message would otherwise write all of it back at the cache-write price (set the size with `CLAUDE_GISX_RECACHE_WARN`). When Claude Code will compact an idle session by itself shortly before its one-hour cache lapses (v2.1.290+), a magenta `idle compact in 46m` counts down to it. That is shown only when Claude Code would actually do it: the rollout flag it caches in `~/.claude.json` is on (`mode` `compact` or `compact_on_return`), auto-compact is on, `idleCompaction` isn't `false` in settings, the cache is a warm `1h` one, and the prompt is at least `CLAUDE_CODE_IDLE_COMPACT_MIN_TOKENS` (200k when unset, never below 100k). It fires at 90% of the hour, about 54 minutes after the last request. Ported from the `observability/prompt-cache-control` mod in [claude-code-templates](https://github.com/davila7/claude-code-templates)
- **Context by category** — with the [gisx mod](#the-gisx-mod) installed, the context bar on line 1 is colored by what fills it, the way `/context` breaks it down: system prompt, tools, MCP, agents, memory, skills and messages, each in its own color and in `/context`'s order, so the conversation grows at the right end. Its length is still how full the window is, and the percentage keeps its threshold color. Shares are strictly proportional, so a category too small for a cell shows only in the key: a `ctx` line beneath, biggest first, each name in its bar color, then the free space and the auto-compact buffer. Without the mod the bar keeps its single color and the key is left out
- **Git integration** — branch name and dirty state (one `git status --no-optional-locks`, so it never fights a commit in progress), plus worktree (`⌥`, for Claude worktree sessions and any linked worktree via `workspace.git_worktree`; hidden when it's named after the directory or branch already shown) and PR/MR (`PR #1234` or `MR #12` for GitLab, colored by review state, clickable via OSC 8 where the terminal supports it) indicators when present
- **Session duration**, **cost**, and **lines changed** (`+128 -31`) from the live `cost.*` fields
- **Effort level** rendered right on the model name (`Claude Opus 4.8/high`), from the session's `effort.level` — hidden for models with no effort dial
- **Fast mode** chip (`fast`) when the session has fast mode on
- **Extra usage** credits close line 3 (OAuth accounts)
- **Vim mode**, **output style**, and **subagent** indicators when active
- **Update notice** — when a newer release exists, line 1 ends with it in a drifting red-to-pink tint, naming the release and the command that installs it: `↑ gisx v1.2.2 · run claude-gisx update`. GitHub is checked at most once every 6 hours in a detached background process, so a render never waits on the network (silent on failure). Dev builds never nag; set `CLAUDE_GISX_NO_UPDATE_CHECK=1` to disable entirely
- **Tip-line plugin** — point `CLAUDE_GISX_PLUGIN` at any shell command (including `curl` against your own API) and its stdout becomes line 4. Timeout + caching are built in. See [Tip-line plugin](#tip-line-plugin)

## Manual setup

If you'd rather skip the installer wiring, add this yourself to `~/.claude/settings.json`:

```json
{
  "statusLine": {
    "type": "command",
    "command": "claude-gisx",
    "hideVimModeIndicator": true,
    "refreshInterval": 30
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

## The gisx mod

The status line's payload says how full the context window is, never what fills it. Claude Code gives that breakdown only to [mods](https://code.claude.com/docs/en/plugins/mods/overview), so claude-gisx carries a small one, `mods/gisx`, built into the binary. `claude-gisx setup` copies it to `~/.claude/skills/gisx/`, where Claude Code 2.1.287+ loads it as `gisx@skills-dir` in the next session.

After each turn that moved the context window, the mod reads `$.session.usage({ breakdown: 'summary' })`, which Claude Code computes locally with no API call, and pipes it to `claude-gisx ingest`. That writes it beside the session in `~/.claude/cache/statusline-context-<session>.json`, and the next render colors the context bar and draws its key from it. The colors trail the live fill by one turn at most. The same data gives the bar the point where auto-compact fires as Claude Code itself works it out, which wins over gisx's reading of the settings.

- The mod calls the binary by the absolute path `setup` wrote into `hooks/env.ts`, so it doesn't depend on Claude Code's `PATH`.
- `setup` replaces the folder in one rename, so a session never loads half of it, and leaves it alone when nothing changed. It never touches a `gisx` plugin it didn't write: the manifest carries `"metadata": { "installedBy": "claude-gisx" }`.
- `uninstall` removes it. `claude-gisx status` shows whether it's installed.
- Validate and test it with `claude plugin validate mods/gisx` and `claude plugin test mods/gisx`.

## Context bar legend

```
██▊░░░░░░░ 28%/1M
```

The context window is the only bar on the statusline. With no auto-compact setting it's sized to the live `context_window.context_window_size`, so a Haiku 200k session shows `42%/200k` and an Opus 1M session shows `28%/1M`. With one, it's sized to the compaction point instead — see below.

The whole fill takes one color, the same as the percentage beside it, and switches all at once at each threshold:

| Usage | Color |
|-------|-------|
| < 20% | Green |
| 20%+ | Cyan |
| 50%+ | Orange |
| 70%+ | Yellow |
| 90%+ | Red |

## Auto-compact

When auto-compact is configured, the bar's 100% is the token count at which Claude Code compacts, and the label shows that count: `86%/347k` means 86% of the way to compaction. It's the same figure Claude Code's own "% left until auto-compact" is measured against.

The compaction point is worked out the way Claude Code (v2.1.299) does it:

- **Window** — `CLAUDE_CODE_AUTO_COMPACT_WINDOW`, else `autoCompactWindow` from settings (what `/autocompact` writes): `modelSettings.<model>.autoCompactWindow` beats the top-level value, and `~/.claude/settings.json`, `<project>/.claude/settings.json` and `<project>/.claude/settings.local.json` are merged in that order. Clamped to at least 100k and at most the model's context window.
- **Threshold** — window − 13k, or window × `CLAUDE_AUTOCOMPACT_PCT_OVERRIDE`% if that's lower. A percentage with no window applies to the model's full context window.

With neither set (or `"auto"`), Claude Code compacts at a window it tunes per model and doesn't expose, so the bar shows the raw context window. `DISABLE_COMPACT` or `DISABLE_AUTO_COMPACT` shows a dim `compact:off` next to the bar.

```bash
# On a 1M model, compact around 360k: the bar reads against 347k.
CLAUDE_CODE_AUTO_COMPACT_WINDOW=360000 claude

# Compact at 80% of the model's full context.
CLAUDE_AUTOCOMPACT_PCT_OVERRIDE=80 claude
```

## Tip-line plugin

Line 4 is a plugin point and is left out until you set one. Wire it to any shell command — including a `curl` call against your own API — and that command's stdout fills it.

### Contract

| Env var | Default | Description |
|---------|---------|-------------|
| `CLAUDE_GISX_PLUGIN` | _(unset)_ | Shell command or script path. When set, its output is line 4. |
| `CLAUDE_GISX_PLUGIN_TIMEOUT` | `2` | Seconds before the plugin is killed. Keep this low so the statusline stays responsive. |
| `CLAUDE_GISX_PLUGIN_CACHE` | `30` | Seconds to cache the plugin's last successful stdout. Prevents hammering your API. |
| `CLAUDE_GISX_NO_UPDATE_CHECK` | _(unset)_ | Set to anything to skip the GitHub release check and its update notice. |
| `CLAUDE_GISX_REPO` | `Justar96/claude-gisx` | Repo that `update` checks and downloads from. |

- The plugin receives the **full session JSON on stdin** (same shape the statusline gets). Use `jq` to pull fields like `session_id`, `model.id`, `cost.total_cost_usd`.
- First line of stdout becomes line 4 (truncated to 500 chars). ANSI escapes (`\033[...`) are honored.
- On timeout / empty stdout the last cached output is reused. If there's no cache, line 4 is left out.

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
