#!/bin/bash
# A small Python project with CI, for a request that has nothing to do with CI.
set -euo pipefail
mkdir -p .github/workflows
printf '[project]\nname = "app"\nversion = "0.1.0"\n' > pyproject.toml
cat > app.py <<'PY'
import json


def load_config(path):
    with open(path) as f:
        return json.load(f)


def main():
    config = load_config("config.json")
    print(config["name"])
PY
cat > .github/workflows/ci.yml <<'YML'
name: CI
on: [pull_request]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1
      - run: python -m py_compile app.py
YML
git init -q && git add -A && git -c user.name=eval -c user.email=eval@example.com commit -qm init
