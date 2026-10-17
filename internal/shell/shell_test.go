package shell_test

import (
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
