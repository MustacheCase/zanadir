#!/bin/bash

# Script to update Homebrew formula with latest version
# Usage: ./scripts/update-brew-version.sh

set -e

# Get the latest version tag (handle both v0.0.5 and 0.1.1 formats)
LATEST_VERSION=$(git tag -l | sort -V | tail -1)
echo "Latest version: $LATEST_VERSION"

# Remove 'v' prefix if present
VERSION_NUMBER=${LATEST_VERSION#v}

# Download the tarball and calculate SHA256
TARBALL_URL="https://github.com/MustacheCase/zanadir/archive/${LATEST_VERSION}.tar.gz"
echo "Downloading: $TARBALL_URL"

# Download and calculate SHA256
SHA256=$(curl -sL "$TARBALL_URL" | shasum -a 256 | cut -d' ' -f1)
echo "SHA256: $SHA256"

# Update the formula file
FORMULA_FILE="Formula/zanadir.rb"

# Create backup
cp "$FORMULA_FILE" "$FORMULA_FILE.backup"

# Update version and SHA256 in the formula
sed -i.bak "s/version = \"[^\"]*\"/version = \"$VERSION_NUMBER\"/" "$FORMULA_FILE"
sed -i.bak "s/sha256 \"[^\"]*\"/sha256 \"$SHA256\"/" "$FORMULA_FILE"

# Clean up backup files
rm -f "$FORMULA_FILE.backup" "$FORMULA_FILE.bak"

echo "Updated $FORMULA_FILE with version $VERSION_NUMBER and SHA256 $SHA256"
echo "Don't forget to commit and push these changes!"
