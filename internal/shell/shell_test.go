package shell_test

import (
	"strings"
	"testing"

	"github.com/MustacheCase/zanadir/internal/shell"
	"github.com/stretchr/testify/assert"
)

func TestCommands(t *testing.T) {
	assert.Equal(t, []string{"set -e", "make lint", "true", "cat a", "grype", "go test  \t ./...", "echo done"},
		shell.Commands("set -e\n# gitleaks is disabled for now\nmake lint || true; cat a | grype\n\ngo test \\\n\t ./... && echo done"))
	assert.Nil(t, shell.Commands(" \n "))
}

func TestWords(t *testing.T) {
	assert.Equal(t, []string{"make", "lint"}, shell.Words("sudo CI=1 GOFLAGS=-mod=mod make lint"))
	assert.Empty(t, shell.Words("FOO=bar"))
}

func TestSwallowed(t *testing.T) {
	runs := func(command string) bool { return strings.Contains(command, "gitleaks") }
	for script, want := range map[string]bool{
		"gitleaks detect || true":                               true,
		"gitleaks detect || echo 'leaks found'":                 true,
		"if ! gitleaks detect; then echo found; fi":             true,
		"if gitleaks detect; then echo clean; fi":               true,
		"! gitleaks detect":                                     true,
		"while ! gitleaks detect; do sleep 1; done":             true,
		"set -e\ngitleaks detect --report r.json \\\n  || true": true,
		"gitleaks detect":                                       false,
		"gitleaks detect || exit 1":                             false,
		"gitleaks detect || exit $?":                            false,
		"gitleaks detect || { echo 'leaks found'; exit 1; }":    false,
		"gitleaks detect || false":                              false,
		"gitleaks detect || true\ngitleaks detect --staged":     false,
		"make || gitleaks detect":                               false,
		`if [ "$TOOL" = gitleaks ]; then make; fi`:              false,
		"pip install gitleaks || true":                          false,
		"echo gitleaks || true":                                 false,
		"# gitleaks detect || true":                             false,
		"":                                                      false,
	} {
		assert.Equal(t, want, shell.Swallowed(script, runs), script)
	}
}

func TestPassive(t *testing.T) {
	for command, want := range map[string]bool{
		"chmod +x bin/grype":           true,
		"python -m pip install bandit": true,
		"uv tool install ruff":         true,
		"trivy --version":              true,
		"npm run lint":                 false,
		"trivy fs --version-check .":   false,
		"go run ./cmd/gosec ./...":     false,
	} {
		assert.Equal(t, want, shell.Passive(strings.Fields(command)), command)
	}
}
