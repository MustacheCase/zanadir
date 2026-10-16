package utils

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileHelpers(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.yaml"), nil, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.yml"), nil, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.txt"), nil, 0o600))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "sub.yml"), 0o755))

	assert.True(t, IsFile(filepath.Join(dir, "a.yml")))
	assert.False(t, IsFile(filepath.Join(dir, "sub.yml")), "a directory is not a file")
	assert.False(t, IsFile(filepath.Join(dir, "missing")))
	assert.True(t, IsDir(filepath.Join(dir, "sub.yml")))
	assert.False(t, IsDir(filepath.Join(dir, "a.yml")))

	assert.Equal(t, "b.yaml", FirstFile(dir, "missing", "sub.yml", "b.yaml", "a.yml"))
	assert.Empty(t, FirstFile(dir, "missing"))

	assert.Equal(t, []string{filepath.Join(dir, "a.yml"), filepath.Join(dir, "b.yaml")}, YAMLFiles(dir))
	assert.Nil(t, YAMLFiles(filepath.Join(dir, "missing")))
}
