#!/usr/bin/env bash
set -euo pipefail

# Build standalone binaries with Bun.
#   ./scripts/build.sh                 build all targets (skips ones Bun
#                                      can't cross-compile to from this host)
#   ./scripts/build.sh <target>...     build only the listed targets
#
# Targets: linux-x64 linux-arm64 darwin-arm64 windows-x64
#
# Note: cross-compilation needs Bun to download a runtime for the target
# platform. Older Bun versions (or canary builds) may not have every runtime
# published. CI is the reliable place to build them all — each platform runs
# `./scripts/build.sh <its-own-target>` on its native runner.

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

VERSION=$(node -p "require('./package.json').version" 2>/dev/null \
        || bun -e 'console.log(require("./package.json").version)' 2>/dev/null \
        || echo "dev")

ALL_TARGETS=(linux-x64 linux-arm64 darwin-arm64 windows-x64)
if [ $# -eq 0 ]; then
    SELECTED=("${ALL_TARGETS[@]}")
else
    SELECTED=("$@")
fi

mkdir -p dist
failed=()
built=()

for target in "${SELECTED[@]}"; do
    ext=""
    [[ "$target" == windows-* ]] && ext=".exe"
    out="dist/claude-gisx-${target}${ext}"
    echo "→ building ${target} → ${out}"
    if bun build src/cli.ts \
        --compile \
        --target="bun-${target}" \
        --define "__VERSION__=\"${VERSION}\"" \
        --minify \
        --outfile "${out}" 2>&1; then
        built+=("${target}")
    else
        failed+=("${target}")
        rm -f "${out}"
        echo "  ${target}: skipped (Bun runtime not available — build on a native runner)"
    fi
done

echo ""
if [ ${#built[@]} -gt 0 ]; then
    echo "✓ built: ${built[*]}"
    ls -lh dist/claude-gisx-* 2>/dev/null
fi
if [ ${#failed[@]} -gt 0 ]; then
    echo ""
    echo "skipped: ${failed[*]}"
    echo "  → build these on native CI runners (see .github/workflows/release.yml)"
    exit 1
fi
