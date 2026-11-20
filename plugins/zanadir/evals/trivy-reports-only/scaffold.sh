#!/bin/bash
# A Go service whose dependency scan prints findings but never fails the build.
set -euo pipefail
mkdir -p .github/workflows
cat > go.mod <<'GO'
module example.com/orders

go 1.24
GO
cat > main.go <<'GO'
package main

import "fmt"

func main() { fmt.Println("orders") }
GO
cat > .github/workflows/ci.yml <<'YML'
name: CI
on:
  pull_request:
  push:
    branches: [main]
permissions:
  contents: read
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1
      - uses: actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e # v7.0.0
        with:
          go-version-file: go.mod
      - run: go vet ./...
      - run: go test -coverprofile=coverage.out ./...
      - name: Scan dependencies
        run: trivy fs --scanners vuln .
YML
git init -q && git add -A && git -c user.name=eval -c user.email=eval@example.com commit -qm init
