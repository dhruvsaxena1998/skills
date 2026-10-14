#!/usr/bin/env bash
# Installs the latest skillset release into ~/.local/bin. Needs only curl.
#   curl -fsSL https://raw.githubusercontent.com/dhruvsaxena1998/skills/main/install.sh | bash
set -euo pipefail

if [ "$(uname -s)" != Darwin ]; then
  echo "install.sh: skillset only has macOS builds; use install.ps1 on Windows" >&2
  exit 1
fi

case "$(uname -m)" in
  arm64) arch=arm64 ;;
  x86_64) arch=amd64 ;;
  *) echo "install.sh: no skillset build for $(uname -m)" >&2; exit 1 ;;
esac

dir="$HOME/.local/bin"
mkdir -p "$dir"
echo "downloading skillset_darwin_$arch from github.com..."
# A bar redrawing itself is noise in a log, so only ask for one on a terminal.
if [ -t 2 ]; then progress=--progress-bar; else progress=--silent; fi
# Download beside the target first, so a cut-off download leaves the old binary in place.
curl -fL "$progress" --show-error -o "$dir/skillset.new" \
  "https://github.com/dhruvsaxena1998/skills/releases/latest/download/skillset_darwin_$arch"
chmod +x "$dir/skillset.new"
mv "$dir/skillset.new" "$dir/skillset"

echo "skillset installed in $dir"
case ":$PATH:" in
  *":$dir:"*) ;;
  *) echo "add it to your PATH:  export PATH=\"\$HOME/.local/bin:\$PATH\"" ;;
esac
