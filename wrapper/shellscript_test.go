package wrapper_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func scripts() map[string]string {
	return map[string]string{
		"scripts/strict.sh":  "#!/usr/bin/env bash\nset -euo pipefail\n# lint everything\ngolangci-lint run\ngo test ./...\n",
		"scripts/kept.sh":    "set -e\nruff check . || true\nmypy .\n",
		"scripts/loose.sh":   "#!/bin/sh\ngolangci-lint run\nmake test\necho done\n",
		"scripts/flagged.sh": "failed=0\nif ! ruff check .; then failed=1; fi\nmypy .\n[ $failed = 0 ] || exit 1\n",
		"scripts/shebang.sh": "#!/bin/bash -e\ngitleaks detect\necho done\n",
		"scripts/toggle.sh":  "set -e\nsemgrep scan\nset +e\ntrivy fs .\nset -o errexit\nruff check .\nset +o errexit\nmypy .\necho done\n",
		"scripts/nested.sh":  "set -e\ncd tools\nbash ../scripts/strict.sh\n./run.sh\n",
		"tools/run.sh":       "set -e\nshellcheck *.sh\n",
		"scripts/self.sh":    "set -e\nbash scripts/self.sh\n",
		"Makefile":           "test:\n\tgo test -race ./...\n",
		"scripts/binary.sh":  "set -e\x00\ngolangci-lint run\n",
		"scripts/linted":     "golangci-lint run\n",
		"scripts/bare.sh":    "set -e\nset\ncd ../..\ngolangci-lint run\necho done\n",
	}
}

func TestShellScriptsAreFollowed(t *testing.T) {
	files := scripts()
	strict := []string{"set -euo pipefail", "golangci-lint run", "go test ./..."}
	assert.Equal(t, strict, expand(t, files, "bash scripts/strict.sh"))
	assert.Equal(t, strict, expand(t, files, "./scripts/strict.sh --verbose"))
	assert.Equal(t, strict, expand(t, files, `sh "scripts/strict.sh"`))
	assert.Equal(t, strict, expand(t, files, "source scripts/strict.sh"))
	assert.Equal(t, strict, expand(t, files, ". scripts/strict.sh"))
	assert.Equal(t, []string{"go test -race ./..."}, expand(t, files, `bash -c "make test"`))
	assert.Equal(t, []string{"golangci-lint run"}, expand(t, files, "bash scripts/linted"), "a shell runs a file of any name")
}

// Without errexit only a script's last command decides its status.
func TestAScriptWithoutErrexitCannotFailOnEarlierCommands(t *testing.T) {
	files := scripts()
	assert.Equal(t, []string{"golangci-lint run || true", "make test || true", "go test -race ./... || true", "echo done"},
		expand(t, files, "bash scripts/loose.sh"), "including what a wrapper it calls runs")
	assert.Equal(t, []string{"golangci-lint run", "make test", "go test -race ./...", "echo done"},
		expand(t, files, "bash -e scripts/loose.sh"))
	assert.Equal(t, []string{"golangci-lint run", "make test", "go test -race ./...", "echo done"},
		expand(t, files, "bash -o errexit scripts/loose.sh"))
	assert.Equal(t, []string{"gitleaks detect", "echo done"}, expand(t, files, "./scripts/shebang.sh"))
	assert.Equal(t, []string{"set -e", "semgrep scan", "set +e || true", "trivy fs . || true", "set -o errexit", "ruff check .", "set +o errexit || true", "mypy . || true", "echo done"},
		expand(t, files, "bash scripts/toggle.sh"))
}

// A condition's failure is the script's to handle, errexit or not.
func TestConditionsAreLeftToTheScript(t *testing.T) {
	assert.Equal(t, []string{"failed=0 || true", "if ! ruff check .; then failed=1; fi", "mypy . || true", "[ $failed = 0 ] || exit 1"},
		expand(t, scripts(), "bash scripts/flagged.sh"))
}

func TestAScriptKeepsItsOwnOrTrue(t *testing.T) {
	assert.Equal(t, []string{"set -e", "ruff check . || true", "mypy ."}, expand(t, scripts(), "bash scripts/kept.sh"))
}

func TestABareSetChangesNothing(t *testing.T) {
	assert.Equal(t, []string{"set -e", "set", "golangci-lint run", "echo done"}, expand(t, scripts(), "bash scripts/bare.sh"),
		"and a cd out of the repository is not followed")
}

func TestScriptsNestAndChangeDirectory(t *testing.T) {
	assert.Equal(t, []string{"set -e", "bash ../scripts/strict.sh", "set -euo pipefail", "golangci-lint run", "go test ./...", "./run.sh", "set -e", "shellcheck *.sh"},
		expand(t, scripts(), "bash scripts/nested.sh"))
	assert.Equal(t, []string{"set -e", "bash scripts/self.sh"}, expand(t, scripts(), "bash scripts/self.sh"), "a script calling itself stops")
}

func TestScriptsZanadirCannotRead(t *testing.T) {
	files := scripts()
	for _, call := range []string{
		"bash scripts/binary.sh",
		"bash scripts/missing.sh",
		"bash ../outside.sh",
		"bash $SCRIPTS/strict.sh",
		"bash scripts/*.sh",
		"bash -x",
		"bash --norc -c",
		"source",
		"./gradlew build",
		"bash --login",
	} {
		assert.Empty(t, expand(t, files, call), call)
	}
}
