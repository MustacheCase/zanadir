#!/bin/bash
# A Node app whose checks run only after a change has merged.
set -euo pipefail
mkdir -p .github/workflows
cat > package.json <<'JSON'
{
  "name": "storefront",
  "version": "1.0.0",
  "scripts": { "test": "node --test", "lint": "eslint ." },
  "devDependencies": { "eslint": "9.39.1" }
}
JSON
printf 'export const price = (n) => n * 100;\n' > index.js
cat > .github/workflows/ci.yml <<'YML'
name: CI
on:
  push:
    branches: [main]
permissions:
  contents: read
jobs:
  checks:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1
      - run: npm ci
      - run: npm run lint
      - run: npm test
      - run: npm audit --audit-level=high
YML
git init -q && git add -A && git -c user.name=eval -c user.email=eval@example.com commit -qm init
