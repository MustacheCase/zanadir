package parser_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/MustacheCase/zanadir/models"
	"github.com/MustacheCase/zanadir/parser"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCodeScanningDefaultSetupIsACodeQLStep(t *testing.T) {
	p := parser.NewCodeScanningParser()
	root := t.TempDir()
	assert.False(t, p.Exists(root))
	artifacts, err := p.Parse(root)
	require.NoError(t, err)
	assert.Empty(t, artifacts)

	marker := filepath.Join(root, filepath.FromSlash(models.CodeScanningFile))
	require.NoError(t, os.MkdirAll(filepath.Dir(marker), 0o755))
	for languages, run := range map[string]string{
		"actions\ngo\n": "codeql database create --language=actions,go",
		"":              "codeql database create",
	} {
		require.NoError(t, os.WriteFile(marker, []byte(languages), 0o600))
		assert.True(t, p.Exists(root))
		artifacts, err := p.Parse(root)
		require.NoError(t, err)
		require.Len(t, artifacts, 1)
		assert.True(t, artifacts[0].HasTrigger("pull_request"))
		assert.Equal(t, run, artifacts[0].Jobs[0].Run)
	}
}
