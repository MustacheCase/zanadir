#!/bin/bash
# Regenerates each case's MCP mocks from the real zanadir server: every case's
# scaffold.sh builds its fixture in a temporary directory, and the answers of
# `zanadir mcp` to scan, fix and skills become mocks/zanadir/<tool>.md.
# Run it from anywhere after a release that changes zanadir's output:
#   plugins/zanadir/evals/update-mocks.sh [path/to/zanadir]
set -euo pipefail

zanadir=${1:-zanadir}
evals=$(cd "$(dirname "$0")" && pwd)
work=$(mktemp -d)
trap 'rm -rf -- "$work"' EXIT

rpc() {
  local dir=$1
  shift
  {
    printf '%s\n' '{"jsonrpc":"2.0","id":0,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"update-mocks","version":"1"}}}'
    printf '%s\n' '{"jsonrpc":"2.0","method":"notifications/initialized"}'
    printf '%s\n' "$@"
  } | "$zanadir" mcp --dir "$dir"
}

mkdir -p "$evals/mocks/zanadir"
rpc "$work" '{"jsonrpc":"2.0","id":1,"method":"tools/list"}' |
  python3 -I -c 'import json, sys
for line in sys.stdin:
    message = json.loads(line)
    if message.get("id") == 1:
        json.dump(message["result"], sys.stdout, indent=2)
        print()' > "$evals/mocks/zanadir/_tools.json"

for scaffold in "$evals"/*/scaffold.sh; do
  case_dir=$(dirname "$scaffold")
  fixture="$work/$(basename "$case_dir")"
  mkdir -p "$fixture" "$case_dir/mocks/zanadir"
  (cd "$fixture" && bash "$scaffold")
  rpc "$fixture" \
    '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"scan","arguments":{}}}' \
    '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"fix","arguments":{}}}' \
    '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"skills","arguments":{}}}' |
    python3 -I -c 'import json, sys
out, names = sys.argv[1], {1: "scan", 2: "fix", 3: "skills"}
for line in sys.stdin:
    message = json.loads(line)
    if message.get("id") in names:
        result = message["result"]
        with open(f"{out}/{names[message['"'"'id'"'"']]}.md", "w") as f:
            f.write("---\nerror: true\n---\n\n" if result.get("isError") else "")
            f.write(result["content"][0]["text"].rstrip("\n") + "\n")' "$case_dir/mocks/zanadir"
  echo "updated $(basename "$case_dir")"
done
