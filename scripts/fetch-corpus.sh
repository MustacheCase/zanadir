#!/usr/bin/env bash
# Downloads every repository in the corpus manifest at its pinned commit.
# Usage: scripts/fetch-corpus.sh [manifest] [destination]
set -euo pipefail

manifest="${1:-handler/testdata/corpus/repos.txt}"
dest="${2:-${CORPUS_DIR:-.corpus}}"
mkdir -p "$dest"

grep -v '^\s*\(#\|$\)' "$manifest" | while read -r host repo commit; do
  # A branch or tag would move, and the snapshots with it.
  if ! [[ "$commit" =~ ^[0-9a-f]{40}$ ]]; then
    echo "$repo: '$commit' is not a full commit hash; pin a commit, never a branch or tag" >&2
    exit 1
  fi
  dir="$dest/${host}_${repo//\//_}"
  if [ "$(cat "$dir/.corpus-commit" 2>/dev/null)" = "$commit" ]; then
    continue
  fi
  case "$host" in
    github) url="https://codeload.github.com/$repo/tar.gz/$commit" ;;
    bitbucket) url="https://bitbucket.org/$repo/get/$commit.tar.gz" ;;
    *) echo "unknown host $host for $repo" >&2; exit 1 ;;
  esac
  rm -rf "$dir" && mkdir -p "$dir"
  if ! curl -fsSL --retry 3 "$url" | tar -xz -C "$dir" --strip-components=1; then
    echo "$repo at $commit can no longer be downloaded - deleted, renamed or made private?" >&2
    echo "Replace or remove it in $manifest, then run make corpus-update." >&2
    exit 1
  fi
  # zanadir scans git repositories only; the history itself is never read.
  mkdir -p "$dir/.git"
  echo "$commit" > "$dir/.corpus-commit"
  echo "fetched $repo"
done
