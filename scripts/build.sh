#!/usr/bin/env bash
set -euo pipefail

# Build standalone binaries with Go's cross-compiler.
#   ./scripts/build.sh                 build all four targets from any host
#   ./scripts/build.sh <target>...     build only the listed targets
#
# Targets use the naming the installers expect (x64, not amd64):
#   linux-x64 linux-arm64 darwin-arm64 windows-x64

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

# VERSION comes from git tag for releases, falls back to dev locally.
VERSION="${VERSION:-$(git describe --tags --abbrev=0 2>/dev/null | sed 's/^v//' || echo dev)}"

ALL_TARGETS=(linux-x64 linux-arm64 darwin-arm64 windows-x64)
if [ $# -eq 0 ]; then
    SELECTED=("${ALL_TARGETS[@]}")
else
    SELECTED=("$@")
fi

mkdir -p dist
for target in "${SELECTED[@]}"; do
    IFS=- read -r os arch <<< "$target"
    goarch="$arch"
    [[ "$arch" == "x64" ]] && goarch="amd64"
    ext=""
    [[ "$os" == "windows" ]] && ext=".exe"
    out="dist/claude-gisx-${target}${ext}"
    echo "→ building ${target} → ${out}"
    GOOS="$os" GOARCH="$goarch" CGO_ENABLED=0 go build \
        -trimpath \
        -ldflags="-s -w -X main.version=${VERSION}" \
        -o "${out}" .
done

echo
echo "✓ built:"
ls -lh dist/claude-gisx-* 2>/dev/null
