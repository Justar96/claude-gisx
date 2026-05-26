import { existsSync, readFileSync } from "node:fs";
import { homedir } from "node:os";
import { join } from "node:path";

import * as c from "./colors";

const SETTINGS_PATH = join(homedir(), ".claude", "settings.json");

function bullet(label: string, body: string): string {
  return `  ${c.dimgray}•${c.reset} ${c.white}${label}${c.reset}  ${c.dim}${body}${c.reset}`;
}

function detectInstallState(): { active: boolean; otherCommand?: string } {
  if (!existsSync(SETTINGS_PATH)) return { active: false };
  try {
    const s = JSON.parse(readFileSync(SETTINGS_PATH, "utf8"));
    const cmd: string | undefined = s?.statusLine?.command;
    if (!cmd) return { active: false };
    if (/\bclaude-gisx\b/.test(cmd)) return { active: true };
    return { active: false, otherCommand: cmd };
  } catch {
    return { active: false };
  }
}

export function helpScreen(version: string): void {
  const state = detectInstallState();
  console.log();
  console.log(`  ${c.bold}${c.blue}claude-gisx${c.reset} ${c.dim}v${version}${c.reset}`);
  console.log(`  ${c.dimgray}rich, dynamic statusline for Claude Code${c.reset}`);
  console.log();

  // Status badge ---------------------------------------------------------
  if (state.active) {
    console.log(`  ${c.green}✓${c.reset} ${c.green}installed${c.reset} ${c.dim}— restart Claude Code if you just installed${c.reset}`);
  } else if (state.otherCommand) {
    console.log(`  ${c.yellow}!${c.reset} another statusLine is active: ${c.dim}${state.otherCommand}${c.reset}`);
    console.log(`    ${c.dim}run ${c.reset}${c.cyan}claude-gisx setup${c.reset} ${c.dim}to back it up and switch${c.reset}`);
  } else {
    console.log(`  ${c.dimgray}○${c.reset} not installed yet`);
  }
  console.log();

  // Quick start ---------------------------------------------------------
  console.log(`  ${c.bold}Get started${c.reset}`);
  console.log(bullet(
    "claude-gisx setup     ",
    "wire into ~/.claude/settings.json (backs up any existing statusLine)",
  ));
  console.log(bullet(
    "claude-gisx status    ",
    "show current install state and backup",
  ));
  console.log(bullet(
    "claude-gisx uninstall ",
    "restore your previous statusLine",
  ));
  console.log(bullet(
    "claude-gisx help      ",
    "this screen",
  ));
  console.log();

  // Manual setup --------------------------------------------------------
  console.log(`  ${c.bold}Or wire it manually${c.reset}`);
  console.log(`  ${c.dim}add to ~/.claude/settings.json${c.reset}`);
  console.log();
  console.log(`    ${c.dim}{${c.reset}`);
  console.log(`      ${c.cyan}"statusLine"${c.reset}: {`);
  console.log(`        ${c.cyan}"type"${c.reset}: ${c.green}"command"${c.reset},`);
  console.log(`        ${c.cyan}"command"${c.reset}: ${c.green}"claude-gisx"${c.reset}`);
  console.log(`      }`);
  console.log(`    ${c.dim}}${c.reset}`);
  console.log();

  // Configuration hints -------------------------------------------------
  console.log(`  ${c.bold}Configure${c.reset} ${c.dim}(all optional, all env-vars)${c.reset}`);
  console.log(bullet(
    "CLAUDE_AUTOCOMPACT_PCT_OVERRIDE",
    "Claude Code's compact %; statusline shows a red ⚠ when reached",
  ));
  console.log(bullet(
    "CLAUDE_CODE_AUTO_COMPACT_WINDOW",
    "shrink the effective compaction window; badge remaps onto used %",
  ));
  console.log(bullet(
    "DISABLE_COMPACT                ",
    "disables auto-compact; statusline shows a dim compact:off badge",
  ));
  console.log(bullet(
    "CLAUDE_GISX_PLUGIN             ",
    "shell command whose stdout replaces the 3rd line (your own API, etc.)",
  ));
  console.log();

  // Pipe explanation ----------------------------------------------------
  console.log(`  ${c.bold}How it runs${c.reset}`);
  console.log(`  ${c.dim}Claude Code pipes a JSON session blob to ${c.reset}${c.cyan}claude-gisx${c.reset}${c.dim} on stdin.${c.reset}`);
  console.log(`  ${c.dim}You can preview the output by piping JSON manually:${c.reset}`);
  console.log();
  console.log(`    ${c.cyan}echo${c.reset} ${c.green}'{"model":{"display_name":"Opus"},"context_window":{"used_percentage":12}}'${c.reset} ${c.dim}| ${c.reset}${c.cyan}claude-gisx${c.reset}`);
  console.log();

  console.log(`  ${c.dim}docs: ${c.reset}${c.cyan}https://github.com/Justar96/claude-gisx${c.reset}`);
  console.log();
}
