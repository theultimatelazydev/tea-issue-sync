#!/bin/sh
# Download a prebuilt tea-issue-sync binary for this platform.
#
#   curl -fsSL https://raw.githubusercontent.com/theultimatelazydev/tea-issue-sync/main/install.sh | sh
#   ./install.sh /custom/path                  install to a specific path
#   TEA_ISSUE_SYNC_DIR=/usr/local/bin ./install.sh   install to a specific dir
#   TEA_ISSUE_SYNC_BASE_URL=... ./install.sh   download from a different release base
#
# Installs to ~/.local/bin by default (no sudo). Requires curl.
set -eu

REPO="theultimatelazydev/tea-issue-sync"
BASE="${TEA_ISSUE_SYNC_BASE_URL:-https://github.com/$REPO/releases/latest/download}"
DIR="${TEA_ISSUE_SYNC_DIR:-$HOME/.local/bin}"
DEST="${1:-$DIR/tea-issue-sync}"

os=$(uname -s | tr '[:upper:]' '[:lower:]')
arch=$(uname -m)
case "$arch" in
	x86_64 | amd64) arch="amd64" ;;
	aarch64 | arm64) arch="arm64" ;;
	*) echo "unsupported architecture: $arch" >&2; exit 1 ;;
esac

mkdir -p "$(dirname "$DEST")"
asset="tea-issue-sync-$os-$arch"
echo "downloading $BASE/$asset -> $DEST"
if ! curl -fL -o "$DEST" "$BASE/$asset"; then
	echo "download failed (is there a release with $asset?)" >&2
	exit 1
fi
chmod +x "$DEST"
echo "installed: $("$DEST" --version)"

# Nudge if the install dir isn't on PATH (the usual "command not found" cause).
case ":$PATH:" in
	*":$(dirname "$DEST"):"*) : ;;
	*) echo "note: $(dirname "$DEST") is not on your PATH — add it, e.g.:"
	   echo "      echo 'export PATH=\"$(dirname "$DEST"):\$PATH\"' >> ~/.zshrc" ;;
esac
