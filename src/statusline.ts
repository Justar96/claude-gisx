import { execSync, spawnSync } from "node:child_process";
import { existsSync, mkdirSync, readFileSync, statSync, writeFileSync } from "node:fs";
import { homedir } from "node:os";
import { basename, join } from "node:path";

import * as c from "./colors";

interface SessionInput {
  model?: { id?: string; display_name?: string };
  workspace?: { current_dir?: string };
  cwd?: string;
  context_window?: {
    used_percentage?: number | null;
    context_window_size?: number;
  };
  exceeds_200k_tokens?: boolean;
  cost?: {
    total_cost_usd?: number;
    total_duration_ms?: number;
  };
  thinking?: { enabled?: boolean };
  effort?: { level?: string };
  output_style?: { name?: string };
  vim?: { mode?: string };
  agent?: { name?: string };
  pr?: { number?: number; review_state?: string };
  worktree?: { name?: string };
  rate_limits?: {
    five_hour?: { used_percentage?: number; resets_at?: number };
    seven_day?: { used_percentage?: number; resets_at?: number };
  };
  session_id?: string;
}

const BAR_WIDTH = 15;

function clamp(n: number, lo: number, hi: number): number {
  return Math.max(lo, Math.min(hi, n));
}

function ctxBar(pctUsed: number): string {
  const p = clamp(pctUsed, 0, 100);
  const filled = Math.floor((p * BAR_WIDTH) / 100);
  let out = "";
  for (let i = 0; i < BAR_WIDTH; i++) {
    if (i < filled) {
      const pos = ((i + 1) * 100) / BAR_WIDTH;
      let col: string;
      if (pos <= 20) col = c.green;
      else if (pos <= 47) col = c.cyan;
      else if (pos <= 67) col = c.orange;
      else if (pos <= 87) col = c.yellow;
      else col = c.red;
      out += `${col}█`;
    } else {
      out += `${c.dimgray}░`;
    }
  }
  return `${out}${c.reset}`;
}

function fmtRemaining(epoch?: number): string {
  if (!epoch) return "";
  const diff = epoch - Math.floor(Date.now() / 1000);
  if (diff <= 0) return "now";
  const d = Math.floor(diff / 86400);
  const h = Math.floor((diff % 86400) / 3600);
  const m = Math.floor((diff % 3600) / 60);
  if (d > 0) return h > 0 ? `${d}d ${h}h` : `${d}d`;
  if (h > 0) return m > 0 ? `${h}h ${m}m` : `${h}h`;
  if (m > 0) return `${m}m`;
  return `${diff}s`;
}

function fmtDuration(ms?: number): string {
  if (!ms) return "";
  const el = Math.floor(ms / 1000);
  if (el >= 3600) return `${Math.floor(el / 3600)}h${Math.floor((el % 3600) / 60)}m`;
  if (el >= 60) return `${Math.floor(el / 60)}m`;
  return `${el}s`;
}

function fmtCtxLabel(size: number): string {
  if (size >= 1_000_000) return `${Math.floor(size / 1_000_000)}M`;
  if (size >= 1000) return `${Math.floor(size / 1000)}k`;
  return `${size}`;
}

function gitInfo(cwd: string): { branch: string; dirty: string } {
  try {
    execSync("git rev-parse --is-inside-work-tree", {
      cwd,
      stdio: ["ignore", "ignore", "ignore"],
    });
  } catch {
    return { branch: "", dirty: "" };
  }
  let branch = "";
  try {
    branch = execSync("git symbolic-ref --short HEAD", {
      cwd,
      encoding: "utf8",
      stdio: ["ignore", "pipe", "ignore"],
    }).trim();
  } catch {}
  let dirty = "";
  try {
    const status = execSync("git status --porcelain", {
      cwd,
      encoding: "utf8",
      stdio: ["ignore", "pipe", "ignore"],
    }).trim();
    if (status) dirty = "*";
  } catch {}
  return { branch, dirty };
}

function readSettings(): Record<string, unknown> {
  const p = join(homedir(), ".claude", "settings.json");
  try {
    return JSON.parse(readFileSync(p, "utf8"));
  } catch {
    return {};
  }
}

function getOauthToken(): string {
  if (process.env.CLAUDE_CODE_OAUTH_TOKEN) return process.env.CLAUDE_CODE_OAUTH_TOKEN;
  const p = join(homedir(), ".claude", ".credentials.json");
  try {
    const json = JSON.parse(readFileSync(p, "utf8"));
    const t = json?.claudeAiOauth?.accessToken;
    if (typeof t === "string" && t) return t;
  } catch {}
  // Try secret-tool (libsecret) as a fallback
  try {
    const res = spawnSync("secret-tool", ["lookup", "service", "Claude Code-credentials"], {
      encoding: "utf8",
      timeout: 2000,
    });
    if (res.status === 0 && res.stdout) {
      const t = JSON.parse(res.stdout)?.claudeAiOauth?.accessToken;
      if (typeof t === "string" && t) return t;
    }
  } catch {}
  return "";
}

interface ExtraUsage {
  used: string;
  limit: string;
}

async function fetchExtraUsage(): Promise<ExtraUsage | null> {
  const cacheDir = "/tmp/claude";
  const cachePath = join(cacheDir, "statusline-extra-cache.json");
  let data: any = null;

  if (existsSync(cachePath)) {
    const ageSec = Math.floor((Date.now() - statSync(cachePath).mtimeMs) / 1000);
    if (ageSec < 60) {
      try {
        data = JSON.parse(readFileSync(cachePath, "utf8"));
      } catch {}
    }
  }

  if (!data) {
    const token = getOauthToken();
    if (token) {
      try {
        const controller = new AbortController();
        const timer = setTimeout(() => controller.abort(), 3000);
        const resp = await fetch("https://api.anthropic.com/api/oauth/usage", {
          headers: {
            Accept: "application/json",
            Authorization: `Bearer ${token}`,
            "anthropic-beta": "oauth-2025-04-20",
          },
          signal: controller.signal,
        });
        clearTimeout(timer);
        if (resp.ok) {
          const body = (await resp.json()) as { extra_usage?: any };
          if (body && body.extra_usage) {
            data = body;
            try {
              mkdirSync(cacheDir, { recursive: true });
              writeFileSync(cachePath, JSON.stringify(body));
            } catch {}
          }
        }
      } catch {}
    }
    if (!data && existsSync(cachePath)) {
      try {
        data = JSON.parse(readFileSync(cachePath, "utf8"));
      } catch {}
    }
  }

  if (!data?.extra_usage?.is_enabled) return null;
  const used = (data.extra_usage.used_credits ?? 0) / 100;
  const limit = (data.extra_usage.monthly_limit ?? 0) / 100;
  return { used: used.toFixed(2), limit: limit.toFixed(2) };
}

function runPlugin(stdinJson: string): string {
  const cmd = process.env.CLAUDE_GISX_PLUGIN;
  if (!cmd) return "";
  const timeoutSec = Number(process.env.CLAUDE_GISX_PLUGIN_TIMEOUT ?? "2") || 2;
  const cacheSec = Number(process.env.CLAUDE_GISX_PLUGIN_CACHE ?? "30") || 30;
  const cacheDir = "/tmp/claude";
  const cachePath = join(cacheDir, "statusline-plugin-cache.txt");

  if (existsSync(cachePath)) {
    const ageSec = Math.floor((Date.now() - statSync(cachePath).mtimeMs) / 1000);
    if (ageSec < cacheSec) {
      try {
        return readFileSync(cachePath, "utf8");
      } catch {}
    }
  }

  let output = "";
  try {
    const res = spawnSync("bash", ["-c", cmd], {
      input: stdinJson,
      encoding: "utf8",
      timeout: timeoutSec * 1000,
      stdio: ["pipe", "pipe", "ignore"],
    });
    if (res.status === 0 && res.stdout) {
      output = res.stdout.split("\n")[0].slice(0, 500);
    }
  } catch {}

  if (output) {
    try {
      mkdirSync(cacheDir, { recursive: true });
      writeFileSync(cachePath, output);
    } catch {}
    return output;
  }
  if (existsSync(cachePath)) {
    try {
      return readFileSync(cachePath, "utf8");
    } catch {}
  }
  return "";
}

interface CompactState {
  pct: number | null;        // mapped threshold on used_percentage's scale, null if no badge
  userSet: boolean;          // user has configured at least one of the relevant env vars
  off: boolean;              // DISABLE_COMPACT is in effect
}

function computeCompactState(ctxSize: number): CompactState {
  const disable = process.env.DISABLE_COMPACT;
  if (disable && !["", "0", "false", "False", "FALSE"].includes(disable)) {
    return { pct: null, userSet: false, off: true };
  }

  const gisxOverride = process.env.CLAUDE_GISX_COMPACT_PCT;
  if (gisxOverride) {
    const p = parseInt(gisxOverride, 10);
    if (!isNaN(p) && p > 0) return { pct: p, userSet: true, off: false };
  }

  const pctOverride = process.env.CLAUDE_AUTOCOMPACT_PCT_OVERRIDE;
  const winOverride = process.env.CLAUDE_CODE_AUTO_COMPACT_WINDOW;
  if (pctOverride || winOverride) {
    const p = parseInt(pctOverride ?? "95", 10);
    const w = winOverride ? parseInt(winOverride, 10) : 0;
    if (w > 0 && ctxSize > 0 && w < ctxSize) {
      return { pct: Math.floor((p * w) / ctxSize), userSet: true, off: false };
    }
    return { pct: p, userSet: true, off: false };
  }

  return { pct: null, userSet: false, off: false };
}

function tipLine(
  pctUsed: number,
  fivePct: number,
  weekPct: number,
  compact: CompactState,
  pluginOutput: string,
): string {
  if (compact.userSet && compact.pct !== null && compact.pct > 0 && pctUsed >= compact.pct) {
    return (
      `${c.dimgray}auto-compact imminent${c.reset} ${c.dim}·${c.reset} ` +
      `${c.dimgray}/compact${c.reset} ${c.dim}now${c.reset} ${c.dim}·${c.reset} ` +
      `${c.dimgray}/clear${c.reset} ${c.dim}reset${c.reset}`
    );
  }
  if (fivePct >= 80 || weekPct >= 80) {
    return (
      `${c.dimgray}rate limit high${c.reset} ${c.dim}·${c.reset} ` +
      `${c.dimgray}consider pausing or switching models${c.reset}`
    );
  }
  if (pluginOutput) return pluginOutput;
  if (pctUsed >= 70) {
    return (
      `${c.dimgray}/compact${c.reset} ${c.dim}free context${c.reset} ${c.dim}·${c.reset} ` +
      `${c.dimgray}/clear${c.reset} ${c.dim}reset session${c.reset}`
    );
  }
  if (pctUsed >= 40) {
    return (
      `${c.dimgray}/compact${c.reset} ${c.dim}free context${c.reset} ${c.dim}·${c.reset} ` +
      `${c.dimgray}shift+tab${c.reset} ${c.dim}interrupt${c.reset} ${c.dim}·${c.reset} ` +
      `${c.dimgray}esc esc${c.reset} ${c.dim}cancel${c.reset}`
    );
  }
  const idx = Math.floor(Date.now() / 1000) % 4;
  switch (idx) {
    case 0:
      return (
        `${c.dimgray}shift+tab${c.reset} ${c.dim}interrupt${c.reset} ${c.dim}·${c.reset} ` +
        `${c.dimgray}/compact${c.reset} ${c.dim}free context${c.reset} ${c.dim}·${c.reset} ` +
        `${c.dimgray}esc esc${c.reset} ${c.dim}cancel${c.reset}`
      );
    case 1:
      return (
        `${c.dimgray}ctrl+r${c.reset} ${c.dim}retry${c.reset} ${c.dim}·${c.reset} ` +
        `${c.dimgray}#${c.reset} ${c.dim}add files${c.reset} ${c.dim}·${c.reset} ` +
        `${c.dimgray}/cost${c.reset} ${c.dim}session cost${c.reset}`
      );
    case 2:
      return (
        `${c.dimgray}/init${c.reset} ${c.dim}setup CLAUDE.md${c.reset} ${c.dim}·${c.reset} ` +
        `${c.dimgray}/review${c.reset} ${c.dim}review changes${c.reset} ${c.dim}·${c.reset} ` +
        `${c.dimgray}/help${c.reset} ${c.dim}commands${c.reset}`
      );
    default:
      return (
        `${c.dimgray}/model${c.reset} ${c.dim}switch model${c.reset} ${c.dim}·${c.reset} ` +
        `${c.dimgray}/vim${c.reset} ${c.dim}vim mode${c.reset} ${c.dim}·${c.reset} ` +
        `${c.dimgray}/config${c.reset} ${c.dim}settings${c.reset}`
      );
  }
}

export async function renderStatusline(stdinJson: string): Promise<void> {
  if (!stdinJson.trim()) {
    process.stdout.write("Claude");
    return;
  }

  let data: SessionInput;
  try {
    data = JSON.parse(stdinJson);
  } catch {
    process.stdout.write("Claude");
    return;
  }

  const modelName = data.model?.display_name || "Claude";
  const pctUsed = Math.floor(data.context_window?.used_percentage ?? 0);
  const ctxSize = data.context_window?.context_window_size ?? 200_000;
  const exceeds200k = data.exceeds_200k_tokens === true;
  const cwd = data.workspace?.current_dir || data.cwd || process.cwd();
  const durationMs = data.cost?.total_duration_ms ?? 0;
  const costUsd = data.cost?.total_cost_usd ?? 0;
  const has1M = ctxSize > 200_000;
  const ctxLabel = fmtCtxLabel(ctxSize);

  // thinking + effort: prefer live JSON, fall back to settings.json
  let thinkingOn = data.thinking?.enabled === true;
  let effortLevel = data.effort?.level || "";
  if (data.thinking === undefined || data.effort === undefined) {
    const settings = readSettings();
    if (data.thinking === undefined && settings.alwaysThinkingEnabled === true) thinkingOn = true;
    if (!effortLevel && typeof settings.effortLevel === "string") effortLevel = settings.effortLevel;
  }

  const compact = computeCompactState(ctxSize);
  const { branch, dirty } = gitInfo(cwd);
  const dir = basename(cwd);
  const dur = fmtDuration(durationMs);

  // ── Line 1: model + bar + workspace + session meta ────────────────────
  let L = `${c.bold}${c.blue}${modelName}${c.reset}`;
  L += `${c.sep}${ctxBar(pctUsed)} ${c.pct(pctUsed)}${pctUsed}%${c.reset}${c.dim}/${c.reset}${c.dimgray}${ctxLabel}${c.reset}`;

  if (compact.userSet && compact.pct !== null && compact.pct > 0 && pctUsed >= compact.pct) {
    L += ` ${c.bold}${c.red}⚠ compact ${compact.pct}%${c.reset}`;
  }
  if (compact.off) L += ` ${c.dimgray}compact:off${c.reset}`;
  if (has1M && exceeds200k) L += ` ${c.dimgray}+ext${c.reset}`;

  L += `${c.sep}${c.cyan}${dir}${c.reset}`;
  if (branch) L += `${c.dim}:${c.reset}${c.green}${branch}${c.red}${dirty}${c.reset}`;

  if (data.worktree?.name) L += `${c.sep}${c.magenta}⌥ ${data.worktree.name}${c.reset}`;
  if (dur) L += `${c.sep}${c.white}${dur}${c.reset}`;

  if (costUsd > 0) {
    const cost = costUsd.toFixed(2);
    if (cost !== "0.00") L += `${c.sep}${c.white}$${cost}${c.reset}`;
  }

  L += thinkingOn
    ? `${c.sep}${c.magenta}● think${c.reset}`
    : `${c.sep}${c.dim}○ think${c.reset}`;

  if (effortLevel) {
    const e = effortLevel;
    if (e === "max" || e === "xhigh") L += `${c.sep}${c.red}▲ ${e}${c.reset}`;
    else if (e === "high") L += `${c.sep}${c.cyan}▲ ${e}${c.reset}`;
    else if (e === "low") L += `${c.sep}${c.dimgray}▽ ${e}${c.reset}`;
    else L += `${c.sep}${c.dim}◆ ${e}${c.reset}`;
  }

  if (data.output_style?.name && data.output_style.name !== "default") {
    L += `${c.sep}${c.dimgray}style:${c.reset}${c.cyan}${data.output_style.name}${c.reset}`;
  }
  if (data.vim?.mode) L += `${c.sep}${c.yellow}${data.vim.mode}${c.reset}`;
  if (data.pr?.number) {
    const state = data.pr.review_state;
    const col =
      state === "approved" ? c.green :
      state === "changes_requested" ? c.red :
      state === "draft" ? c.dimgray :
      c.yellow;
    L += `${c.sep}${col}PR #${data.pr.number}${c.reset}`;
  }
  if (data.agent?.name) L += `${c.sep}${c.magenta}@${data.agent.name}${c.reset}`;

  // ── Line 2: rate limits + extra credits ────────────────────────────────
  let R = "";
  const fivePct = Math.floor(data.rate_limits?.five_hour?.used_percentage ?? 0);
  const weekPct = Math.floor(data.rate_limits?.seven_day?.used_percentage ?? 0);
  const hasRateLimits = data.rate_limits !== undefined;

  if (hasRateLimits) {
    const fiveRem = 100 - fivePct;
    const weekRem = 100 - weekPct;
    R += `${c.white}5h${c.reset} ${c.pct(fivePct)}${fivePct}%${c.reset}`;
    const fiveReset = fmtRemaining(data.rate_limits?.five_hour?.resets_at);
    if (fiveReset) R += ` ${c.dimgray}resets${c.reset} ${c.remaining(fiveRem)}${fiveReset}${c.reset}`;

    R += `${c.sep}${c.white}7d${c.reset} ${c.pct(weekPct)}${weekPct}%${c.reset}`;
    const weekReset = fmtRemaining(data.rate_limits?.seven_day?.resets_at);
    if (weekReset) R += ` ${c.dimgray}resets${c.reset} ${c.remaining(weekRem)}${weekReset}${c.reset}`;

    const extra = await fetchExtraUsage();
    if (extra) {
      R += `${c.sep}${c.dimgray}extra${c.reset} ${c.white}$${extra.used}${c.dimgray}/${c.reset}${c.white}$${extra.limit}${c.reset}`;
    }
  }

  // ── Line 3: plugin output or built-in tip ──────────────────────────────
  const pluginOutput = runPlugin(stdinJson);
  const T = tipLine(pctUsed, fivePct, weekPct, compact, pluginOutput);

  process.stdout.write(L + "\n");
  if (R) process.stdout.write(R + "\n");
  if (T) process.stdout.write(T);
}
