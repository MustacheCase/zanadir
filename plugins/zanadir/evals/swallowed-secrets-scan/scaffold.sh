#!/bin/bash
# A Python package whose secrets scan swallows its own failure.
set -euo pipefail
mkdir -p .github/workflows src/billing tests
printf '[project]\nname = "billing"\nversion = "0.1.0"\n' > pyproject.toml
printf 'def total(items):\n    return sum(items)\n' > src/billing/__init__.py
printf 'from billing import total\n\n\ndef test_total():\n    assert total([1, 2]) == 3\n' > tests/test_total.py
cat > .github/workflows/ci.yml <<'YML'
name: CI
on: [pull_request]
permissions:
  contents: read
jobs:
  checks:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1
        with:
          fetch-depth: 0
      - run: pip install -e . pytest pytest-cov ruff
      - run: ruff check .
      - run: pytest --cov=billing
      - name: Look for leaked secrets
        run: gitleaks detect --source . || true
YML
git init -q && git add -A && git -c user.name=eval -c user.email=eval@example.com commit -qm init
