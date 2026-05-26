#!/usr/bin/env bun
import { renderStatusline } from "./statusline";
import { installCmd, statusCmd, uninstallCmd } from "./install";
import { helpScreen } from "./help";

// Bun's --compile bakes this string in at build time via a build-time replace.
declare const __VERSION__: string;
const VERSION = typeof __VERSION__ !== "undefined" ? __VERSION__ : "dev";

function readStdinSync(): string {
  if (process.stdin.isTTY) return "";
  try {
    const chunks: Buffer[] = [];
    const fd = 0;
    const buf = Buffer.alloc(65536);
    const fs = require("node:fs");
    for (;;) {
      let n = 0;
      try {
        n = fs.readSync(fd, buf, 0, buf.length, null);
      } catch (e: any) {
        if (e && (e.code === "EAGAIN" || e.code === "EWOULDBLOCK")) break;
        throw e;
      }
      if (!n) break;
      chunks.push(Buffer.from(buf.subarray(0, n)));
    }
    return Buffer.concat(chunks).toString("utf8");
  } catch {
    return "";
  }
}

function parseFlags(args: string[]): { positional: string[]; flags: Set<string> } {
  const positional: string[] = [];
  const flags = new Set<string>();
  for (const a of args) {
    if (a.startsWith("--")) flags.add(a.slice(2));
    else positional.push(a);
  }
  return { positional, flags };
}

async function main(): Promise<number> {
  const { positional, flags } = parseFlags(process.argv.slice(2));
  const cmd = positional[0];

  // A subcommand wins outright — never touch stdin, which can block when the
  // caller (e.g. `curl | bash` running install.sh) hands us an inherited pipe.
  if (cmd) {
    switch (cmd) {
      case "help":
      case "--help":
      case "-h":
        helpScreen(VERSION);
        return 0;
      case "version":
      case "--version":
      case "-v":
        console.log(VERSION);
        return 0;
      case "setup":
        return installCmd({ force: flags.has("force"), noCheck: flags.has("no-check") });
      case "uninstall":
        return uninstallCmd({ force: flags.has("force") });
      case "status":
        return statusCmd();
      default:
        process.stderr.write(`unknown command: ${cmd}\n`);
        process.stderr.write(`run 'claude-gisx help' for usage\n`);
        return 1;
    }
  }

  // No args: if stdin is piped (Claude Code's statusline call), render from
  // it; otherwise show the help screen.
  if (!process.stdin.isTTY) {
    const stdin = readStdinSync();
    if (stdin.trim()) {
      await renderStatusline(stdin);
      return 0;
    }
  }
  helpScreen(VERSION);
  return 0;
}

main()
  .then((code) => process.exit(code))
  .catch((err) => {
    process.stderr.write(`${err?.stack || err}\n`);
    process.exit(1);
  });
