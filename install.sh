#!/usr/bin/env bash
# claude-gisx installer (Linux + macOS)
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/Justar96/claude-gisx/main/install.sh | bash
#
# Options (env vars):
#   CLAUDE_GISX_VERSION=v1.2.3       pin to a specific release tag
#   CLAUDE_GISX_INSTALL_DIR=/path    override install dir (default: ~/.local/bin)
#   CLAUDE_GISX_REPO=owner/repo      pull binaries from a fork
#   CLAUDE_GISX_SKIP_SETUP=1         install the binary but skip writing settings.json
#
# Exit codes:
#   0 success · 1 missing tool · 2 unsupported OS/arch · 3 download failed · 4 setup failed

set -euo pipefail

REPO="${CLAUDE_GISX_REPO:-Justar96/claude-gisx}"
TAG="${CLAUDE_GISX_VERSION:-latest}"
INSTALL_DIR="${CLAUDE_GISX_INSTALL_DIR:-$HOME/.local/bin}"
SKIP_SETUP="${CLAUDE_GISX_SKIP_SETUP:-}"

# ── pretty output ─────────────────────────────────────────────────────────
if [ -t 1 ]; then
    bold=$'\033[1m'; dim=$'\033[2m'; rst=$'\033[0m'
    green=$'\033[38;2;0;175;80m'; red=$'\033[38;2;255;85;85m'
    blue=$'\033[38;2;0;153;255m'; gray=$'\033[38;2;120;120;120m'
else
    bold=""; dim=""; rst=""; green=""; red=""; blue=""; gray=""
fi
ok="${green}✓${rst}"
err="${red}✗${rst}"

say()  { printf "  %s\n" "$*"; }
note() { printf "  ${dim}%s${rst}\n" "$*"; }
fail() { printf "  %s ${red}%s${rst}\n" "$err" "$*" >&2; exit "${2:-1}"; }

# ── tool check ────────────────────────────────────────────────────────────
need() { command -v "$1" >/dev/null 2>&1 || fail "missing required tool: $1" 1; }
need curl
need uname
need mkdir
need install
need mv
need rm

# ── detect OS / arch ──────────────────────────────────────────────────────
detect_target() {
    local os arch
    os=$(uname -s | tr '[:upper:]' '[:lower:]')
    arch=$(uname -m)
    case "$os" in
        linux)  os="linux"  ;;
        darwin) os="darwin" ;;
        *) fail "unsupported OS: $os (this installer supports linux and darwin)" 2 ;;
    esac
    case "$arch" in
        x86_64|amd64) arch="x64"   ;;
        aarch64|arm64) arch="arm64" ;;
        *) fail "unsupported architecture: $arch" 2 ;;
    esac
    printf "%s-%s" "$os" "$arch"
}

# ── resolve download URL ──────────────────────────────────────────────────
resolve_tag() {
    if [ "$TAG" = "latest" ]; then
        local api="https://api.github.com/repos/${REPO}/releases/latest"
        local resolved
        resolved=$(curl -fsSL "$api" 2>/dev/null \
            | grep -oE '"tag_name"[[:space:]]*:[[:space:]]*"[^"]+"' \
            | head -1 \
            | sed 's/.*"\([^"]*\)"$/\1/' || true)
        [ -n "$resolved" ] || fail "could not resolve latest release tag from ${api}" 3
        echo "$resolved"
    else
        echo "$TAG"
    fi
}

printf "\n  %s%sclaude-gisx%s installer\n  %s%s%s\n\n" "$bold" "$blue" "$rst" "$dim" "github.com/${REPO}" "$rst"

TARGET=$(detect_target)
say "platform   ${bold}${TARGET}${rst}"

TAG=$(resolve_tag)
say "version    ${bold}${TAG}${rst}"

ASSET="claude-gisx-${TARGET}"
URL="https://github.com/${REPO}/releases/download/${TAG}/${ASSET}"
say "source     ${dim}${URL}${rst}"
say "install to ${dim}${INSTALL_DIR}${rst}"
echo

# ── download ──────────────────────────────────────────────────────────────
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

say "downloading…"
if ! curl -fSL --progress-bar "$URL" -o "${TMP}/${ASSET}"; then
    fail "download failed from ${URL}" 3
fi

# Verify checksum if SHA256SUMS is available
SUMS_URL="https://github.com/${REPO}/releases/download/${TAG}/SHA256SUMS"
if curl -fsSL "$SUMS_URL" -o "${TMP}/SHA256SUMS" 2>/dev/null; then
    say "verifying checksum…"
    expected=$(grep " ${ASSET}\$" "${TMP}/SHA256SUMS" | awk '{print $1}' || true)
    if [ -n "$expected" ]; then
        if command -v sha256sum >/dev/null 2>&1; then
            actual=$(sha256sum "${TMP}/${ASSET}" | awk '{print $1}')
        else
            actual=$(shasum -a 256 "${TMP}/${ASSET}" | awk '{print $1}')
        fi
        [ "$expected" = "$actual" ] || fail "checksum mismatch for ${ASSET}" 3
        say "${ok} checksum verified"
    fi
fi

# ── install ───────────────────────────────────────────────────────────────
mkdir -p "${INSTALL_DIR}"
install -m 0755 "${TMP}/${ASSET}" "${INSTALL_DIR}/claude-gisx"
say "${ok} installed ${dim}${INSTALL_DIR}/claude-gisx${rst}"

# ── PATH hint ─────────────────────────────────────────────────────────────
case ":$PATH:" in
    *":${INSTALL_DIR}:"*) ;;
    *)
        echo
        note "${INSTALL_DIR} is not in your PATH. Add this to your shell profile:"
        printf "    %sexport PATH=\"%s:\$PATH\"%s\n" "$gray" "${INSTALL_DIR}" "$rst"
        ;;
esac

# ── wire into Claude Code ─────────────────────────────────────────────────
if [ -z "$SKIP_SETUP" ]; then
    echo
    say "running ${bold}claude-gisx setup${rst}…"
    if ! "${INSTALL_DIR}/claude-gisx" setup; then
        fail "setup failed — run '${INSTALL_DIR}/claude-gisx setup' manually" 4
    fi
fi

echo
say "${ok} done. ${dim}restart Claude Code to see the new statusline.${rst}"
echo
