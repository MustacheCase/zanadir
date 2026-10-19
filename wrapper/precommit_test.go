package wrapper_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

const precommitConfig = `default_stages: [pre-commit, manual]
repos:
  - repo: https://github.com/gitleaks/gitleaks
    rev: v8.30.1
    hooks:
      - id: gitleaks
  - repo: https://github.com/woodruffw/zizmor-pre-commit
    rev: v1.31.1
    hooks:
      - id: zizmor
        args: [--min-severity, medium]
  - repo: https://github.com/compilerla/conventional-pre-commit
    rev: v4.0.0
    hooks:
      - id: conventional-pre-commit
        stages: [commit-msg]
  - repo: local
    hooks:
      - id: lint
        name: lint
        entry: make lint
        language: system
      - id: slow-scan
        entry: semgrep ci
        language: system
        stages: [manual]
      - id: old-stage-name
        entry: actionlint
        language: system
        stages: [commit]
`

func TestPreCommitFollowsHooks(t *testing.T) {
	files := map[string]string{".pre-commit-config.yaml": precommitConfig, "Makefile": "lint:\n\tgolangci-lint run\n"}

	assert.Equal(t, []string{"gitleaks", "zizmor --min-severity medium", "make lint", "golangci-lint run", "actionlint"},
		expand(t, files, "pre-commit run --all-files"), "commit-msg and manual hooks do not run by default")
	assert.Equal(t, []string{"gitleaks", "zizmor --min-severity medium", "make lint", "golangci-lint run", "semgrep ci"},
		expand(t, files, "prek run --hook-stage manual --all-files"))
	assert.Len(t, expand(t, files, "prek run --hook-stage=manual"), 5)
	assert.Equal(t, []string{"zizmor --min-severity medium"}, expand(t, files, "prek run --show-diff-on-failure --all-files zizmor"),
		"hook ids on the command line run only those hooks")
	assert.Equal(t, []string{"gitleaks"}, expand(t, files, "pre-commit run gitleaks --from-ref origin/main --to-ref HEAD"))
	assert.Equal(t, []string{"gitleaks", "zizmor --min-severity medium", "make lint", "golangci-lint run", "actionlint"},
		expand(t, files, "pre-commit run --files a.py b.py"), "--files lists files, not hooks")
}

func TestPreCommitReadsTheConfigItIsGiven(t *testing.T) {
	files := map[string]string{"ci/hooks.yml": "repos:\n  - repo: local\n    hooks:\n      - id: secrets\n        entry: trufflehog git file://.\n"}
	assert.Equal(t, []string{"trufflehog git file://."}, expand(t, files, "pre-commit run -c ci/hooks.yml --all-files"))
	assert.Equal(t, []string{"trufflehog git file://."}, expand(t, files, "pre-commit run --config=ci/hooks.yml"))
}

func TestPreCommitNeedsARunAndAConfig(t *testing.T) {
	files := map[string]string{".pre-commit-config.yaml": precommitConfig}
	assert.Nil(t, expand(t, files, "pre-commit install"))
	assert.Nil(t, expand(t, files, "pre-commit"))
	assert.Nil(t, expand(t, map[string]string{"README.md": ""}, "pre-commit run --all-files"))
	assert.Nil(t, expand(t, map[string]string{".pre-commit-config.yaml": "repos: [\n"}, "pre-commit run --all-files"))
	assert.Nil(t, expand(t, files, "pre-commit run -c ../outside.yaml"))
}
