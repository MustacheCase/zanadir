package parser_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/MustacheCase/zanadir/parser"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Each of these walked an alias forever before decoding refused them.
func TestAnAliasInsideItsOwnAnchorIsSkipped(t *testing.T) {
	for _, c := range []struct {
		parser parser.Parser
		file   string
		yaml   string
	}{
		{parser.NewGitlabParser(), ".gitlab-ci.yml", "job:\n  script: &a [echo, *a]\n"},
		{parser.NewBitbucketParser(), "bitbucket-pipelines.yml", "pipelines:\n  default: &p\n    - parallel: *p\n"},
		{parser.NewAzureParser(), "azure-pipelines.yml", "jobs:\n  - job: a\n    strategy: &s\n      runOnce: *s\n"},
		{parser.NewAzureParser(), "azure-pipelines.yml", "steps: &a\n  - \"${{ if true }}\": *a\n"},
	} {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, c.file), []byte(c.yaml), 0o600))
		artifacts, err := c.parser.Parse(dir)
		require.NoError(t, err)
		require.Len(t, artifacts, 1, c.file)
		assert.Contains(t, artifacts[0].Skipped, "an alias refers to a node that contains it", c.yaml)
	}
}
