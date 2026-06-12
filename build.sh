#!/bin/sh
# Build a self-contained tea-issue-sync binary (no Node/bun needed to run it).
#
#   ./build.sh          current platform → dist/tea-issue-sync
#   ./build.sh --all    darwin/linux × arm64/x64 → dist/tea-issue-sync-<os>-<arch>
#
# Requires bun (https://bun.sh) as the build tool only.
set -eu
cd "$(dirname "$0")"
mkdir -p dist

if [ "${1:-}" = "--all" ]; then
  for target in bun-darwin-arm64 bun-darwin-x64 bun-linux-x64 bun-linux-arm64; do
    suffix=${target#bun-}
    echo "building dist/tea-issue-sync-$suffix"
    bun build --compile --target="$target" ./tea-issue-sync.mjs --outfile "dist/tea-issue-sync-$suffix"
  done
else
  bun build --compile ./tea-issue-sync.mjs --outfile dist/tea-issue-sync
fi
