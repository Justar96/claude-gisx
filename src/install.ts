import { execSync } from "node:child_process";
import {
  existsSync,
  mkdirSync,
  readFileSync,
  renameSync,
  rmdirSync,
  unlinkSync,
  writeFileSync,
} from "node:fs";
import { homedir } from "node:os";
import { dirname, join } from "node:path";

import * as c from "./colors";

const HOME = homedir();
const CLAUDE_DIR = join(HOME, ".claude");
const SETTINGS = join(CLAUDE_DIR, "settings.json");
const BACKUP_DIR = join(CLAUDE_DIR, ".gisx");
const BACKUP_PREV = join(BACKUP_DIR, "prev-statusline.json");
const BACKUP_FULL = join(BACKUP_DIR, "settings.backup.json");

const ok = `${c.green}✓${c.reset}`;
const fail = `${c.red}✗${c.reset}`;
const dot = `${c.dimgray}·${c.reset}`;

function readJSON<T = any>(p: string): T | null {
  try {
    const raw = readFileSync(p, "utf8").trim();
    return raw ? (JSON.parse(raw) as T) : null;
  } catch (e: any) {
    if (e?.code === "ENOENT") return null;
    if (e instanceof SyntaxError) {
      process.stderr.write(`  ${fail} invalid JSON: ${p}\n`);
      return null;
    }
    throw e;
  }
}

function writeJSON(p: string, data: unknown): void {
  mkdirSync(dirname(p), { recursive: true });
  const tmp = `${p}.${process.pid}.tmp`;
  try {
    writeFileSync(tmp, JSON.stringify(data, null, 2) + "\n", { mode: 0o644 });
    renameSync(tmp, p);
  } catch (e) {
    try { unlinkSync(tmp); } catch {}
    throw e;
  }
}

function rm(p: string): void {
  try { unlinkSync(p); } catch {}
}

// "Ours" detection — the command path can vary (PATH lookup, absolute path,
// or running from a Bun-compiled binary), so accept anything that ends in
// the claude-gisx binary name.
function isOurs(settings: any): boolean {
  const cmd: string | undefined = settings?.statusLine?.command;
  if (!cmd) return false;
  return /\bclaude-gisx\b/.test(cmd);
}

function saveBackup(settings: any): void {
  const had = "statusLine" in settings;
  writeJSON(BACKUP_FULL, { ts: new Date().toISOString(), settings });
  writeJSON(BACKUP_PREV, {
    ts: new Date().toISOString(),
    had,
    value: had ? settings.statusLine : null,
  });
}

function loadPrev(): { found: boolean; had?: boolean; value?: any } {
  const d = readJSON(BACKUP_PREV);
  if (!d) return { found: false };
  return { found: true, had: d.had === true, value: d.value };
}

function cleanBackup(): void {
  rm(BACKUP_PREV);
  rm(BACKUP_FULL);
  try { rmdirSync(BACKUP_DIR); } catch {}
}

function bin(cmd: string): string | null {
  try {
    return execSync(`command -v ${cmd}`, { encoding: "utf8" }).trim();
  } catch {
    return null;
  }
}

function ver(cmd: string, flag = "--version"): string | null {
  try {
    const out = execSync(`${cmd} ${flag} 2>&1`, { encoding: "utf8" }).trim();
    const m = out.match(/(\d+\.\d+[\.\d]*)/);
    return m ? m[1] : null;
  } catch {
    return null;
  }
}

function checkDeps(): boolean {
  // The Bun-compiled binary embeds its own runtime, so we no longer need bash
  // or jq at runtime. git/curl remain optional for git info and OAuth usage.
  const deps = [
    { name: "git", required: false },
    { name: "curl", required: false },
  ];
  let pass = true;
  for (const d of deps) {
    const found = bin(d.name);
    const v = found ? ver(d.name) : null;
    if (found) {
      console.log(`  ${ok} ${d.name}${v ? ` ${c.dimgray}${v}${c.reset}` : ""}`);
    } else if (d.required) {
      console.log(`  ${fail} ${d.name} ${c.red}not found${c.reset}`);
      pass = false;
    } else {
      console.log(`  ${c.dim}- ${d.name} (optional)${c.reset}`);
    }
  }
  return pass;
}

export function banner(): void {
  console.log();
  console.log(`  ${c.bold}${c.blue}claude-gisx${c.reset}`);
  console.log(`  ${c.dimgray}rich statusline for Claude Code${c.reset}`);
  console.log();
}

function preview(): void {
  console.log(`  ${c.dimgray}preview${c.reset}`);
  console.log(
    `  ${c.bold}${c.blue}Claude Opus 4.7${c.reset} ${dot} ` +
    `${c.green}██${c.cyan}██${c.reset}${c.dimgray}░░░░░░░░░░░${c.reset} ` +
    `${c.green}28%${c.dim}/${c.reset}${c.dimgray}1M${c.reset} ${c.dimgray}+ext${c.reset} ${dot} ` +
    `${c.cyan}myproject${c.reset}${c.dim}:${c.reset}${c.green}main${c.reset} ${dot} ` +
    `${c.white}12m${c.reset} ${dot} ${c.white}$1.20${c.reset} ${dot} ` +
    `${c.magenta}● think${c.reset} ${dot} ${c.cyan}▲ high${c.reset}`,
  );
  console.log(
    `  ${c.white}5h${c.reset} ${c.green}12%${c.reset} ${c.dimgray}resets${c.reset} ` +
    `${c.green}4h 2m${c.reset} ${dot} ${c.white}7d${c.reset} ${c.green}3%${c.reset} ` +
    `${c.dimgray}resets${c.reset} ${c.green}6d 1h${c.reset}`,
  );
  console.log(
    `  ${c.dimgray}shift+tab${c.reset} ${c.dim}interrupt${c.reset} ${dot} ` +
    `${c.dimgray}/compact${c.reset} ${c.dim}free context${c.reset} ${dot} ` +
    `${c.dimgray}esc esc${c.reset} ${c.dim}cancel${c.reset}`,
  );
  console.log();
}

interface Options {
  force?: boolean;
  noCheck?: boolean;
}

export function statusCmd(): number {
  const s = readJSON<any>(SETTINGS);
  const prev = loadPrev();
  banner();
  if (!s) console.log(`  config   ${c.dim}missing${c.reset}`);
  else if (isOurs(s)) console.log(`  config   ${ok} active  ${c.dim}${s.statusLine.command}${c.reset}`);
  else if (s.statusLine) console.log(`  config   ${c.yellow}other${c.reset}  ${c.dim}${JSON.stringify(s.statusLine)}${c.reset}`);
  else console.log(`  config   ${c.dim}none${c.reset}`);
  if (prev.found) {
    console.log(
      prev.had
        ? `  backup   ${ok} ${c.dim}${JSON.stringify(prev.value)}${c.reset}`
        : `  backup   ${c.dim}(default)${c.reset}`,
    );
  } else {
    console.log(`  backup   ${c.dim}none${c.reset}`);
  }
  if (existsSync(BACKUP_FULL)) console.log(`  snapshot ${c.dim}${BACKUP_FULL}${c.reset}`);
  console.log();
  return 0;
}

export function uninstallCmd(opts: Options = {}): number {
  banner();
  const s = readJSON<any>(SETTINGS);
  if (!s) {
    console.log(`  ${c.dim}nothing to do${c.reset}`);
    cleanBackup();
    return 0;
  }
  if (s.statusLine && !isOurs(s) && !opts.force) {
    console.log(`  ${fail} statusLine belongs to: ${s.statusLine.command}`);
    console.log(`  ${c.dim}pass --force to override${c.reset}`);
    return 1;
  }
  const prev = loadPrev();
  if (prev.found && prev.had) {
    s.statusLine = prev.value;
    console.log(`  ${ok} restored: ${c.dim}${JSON.stringify(prev.value)}${c.reset}`);
  } else {
    delete s.statusLine;
    console.log(`  ${ok} restored: ${c.dim}default${c.reset}`);
  }
  writeJSON(SETTINGS, s);
  cleanBackup();
  console.log(`  ${ok} cleaned up backups`);
  console.log();
  console.log(`  ${c.dim}restart Claude Code to apply${c.reset}`);
  console.log();
  return 0;
}

export function installCmd(opts: Options = {}): number {
  banner();
  const s = readJSON<any>(SETTINGS) ?? {};

  if (isOurs(s) && !opts.force) {
    console.log(`  ${ok} already active`);
    console.log(`  ${c.dim}--force to reinstall, status to inspect${c.reset}`);
    console.log();
    return 0;
  }

  if (!opts.noCheck) {
    console.log(`  ${c.dimgray}dependencies${c.reset}`);
    const pass = checkDeps();
    console.log();
    if (!pass) {
      console.log(`  ${fail} missing required dependencies`);
      console.log(`  ${c.dim}install them and retry, or --no-check to skip${c.reset}`);
      console.log();
      return 1;
    }
  }

  preview();
  saveBackup(s);
  if (s.statusLine && !isOurs(s)) {
    console.log(`  ${dot} backed up: ${c.dim}${JSON.stringify(s.statusLine)}${c.reset}`);
  }
  s.statusLine = { type: "command", command: "claude-gisx" };
  writeJSON(SETTINGS, s);
  console.log(`  ${ok} installed`);
  console.log();
  console.log(`  ${c.dim}restart Claude Code to apply${c.reset}`);
  console.log();
  return 0;
}
