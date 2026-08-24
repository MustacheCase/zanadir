#!/bin/bash
# Point the Homebrew formula at the latest released tag.
# Usage: ./scripts/update-brew-version.sh [tag]

set -euo pipefail

FORMULA_FILE="Formula/zanadir.rb"
TAG="${1:-$(git tag -l | sort -V | tail -1)}"

if [ -z "$TAG" ]; then
  echo "No tag found; pass one explicitly." >&2
  exit 1
fi

URL="https://github.com/MustacheCase/zanadir/archive/refs/tags/${TAG}.tar.gz"
echo "Tag:  $TAG"
echo "URL:  $URL"

SHA256=$(curl -fsSL "$URL" | shasum -a 256 | cut -d' ' -f1)
if [ -z "$SHA256" ]; then
  echo "Could not download $URL" >&2
  exit 1
fi
echo "SHA:  $SHA256"

sed -i.bak -E "s|^  url \".*\"|  url \"${URL}\"|" "$FORMULA_FILE"
sed -i.bak -E "s|^  sha256 \".*\"|  sha256 \"${SHA256}\"|" "$FORMULA_FILE"
rm -f "$FORMULA_FILE.bak"

echo
echo "Updated $FORMULA_FILE:"
grep -E '^  (url|sha256) ' "$FORMULA_FILE"
echo
echo "The tap is this repository, so committing to main publishes the formula."
