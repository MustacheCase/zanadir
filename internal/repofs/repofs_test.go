package repofs

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadFileStaysInTheRepository(t *testing.T) {
	base := t.TempDir()
	repo, outside := filepath.Join(base, "repo"), filepath.Join(base, "outside")
	require.NoError(t, os.MkdirAll(filepath.Join(repo, "sub"), 0o755))
	require.NoError(t, os.MkdirAll(outside, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(repo, "sub", "in.mk"), []byte("in"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(outside, "out.mk"), []byte("out"), 0o600))
	require.NoError(t, os.Symlink(outside, filepath.Join(repo, "linked")))
	require.NoError(t, os.Symlink("sub", filepath.Join(repo, "inside")))

	data, err := ReadFile(repo, filepath.Join(repo, "sub", "in.mk"))
	require.NoError(t, err)
	assert.Equal(t, "in", string(data))
	data, err = ReadFile(repo, filepath.Join(repo, "inside", "in.mk"))
	require.NoError(t, err, "a relative symlink that stays inside is followed")
	assert.Equal(t, "in", string(data))

	for _, path := range []string{filepath.Join(repo, "..", "outside", "out.mk"), filepath.Join(repo, "linked", "out.mk"), outside} {
		_, err := ReadFile(repo, path)
		assert.Error(t, err, path)
	}
}

func TestPathsAreRelativeToTheWorkingDirectory(t *testing.T) {
	base := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(base, "mk"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(base, "outside.mk"), []byte("out"), 0o600))
	t.Chdir(base)

	_, err := ReadFile("mk", filepath.Join("mk", "..", "outside.mk"))
	assert.ErrorContains(t, err, "outside the repository")
	_, err = ReadFile("mk", filepath.Join(base, "mk", "x"))
	assert.ErrorContains(t, err, "outside the repository", "a path not built from the root is refused")
}

func TestWriteFileStaysInTheRepository(t *testing.T) {
	base := t.TempDir()
	repo, outside := filepath.Join(base, "repo"), filepath.Join(base, "outside")
	require.NoError(t, os.MkdirAll(filepath.Join(repo, ".github"), 0o755))
	require.NoError(t, os.MkdirAll(outside, 0o755))
	require.NoError(t, os.Symlink(outside, filepath.Join(repo, ".github", "workflows")))

	assert.Error(t, WriteFile(repo, filepath.Join(repo, ".github", "workflows", "x.yml"), []byte("x"), 0o644))
	entries, err := os.ReadDir(outside)
	require.NoError(t, err)
	assert.Empty(t, entries, "nothing is written through the symlink")

	require.NoError(t, WriteFile(repo, filepath.Join(repo, "a", "b", "y.yml"), []byte("y"), 0o644))
	data, err := os.ReadFile(filepath.Join(repo, "a", "b", "y.yml"))
	require.NoError(t, err)
	assert.Equal(t, "y", string(data))
	assert.Error(t, WriteFile(repo, filepath.Join(base, "z.yml"), nil, 0o644))
	assert.Error(t, WriteFile(filepath.Join(base, "missing"), filepath.Join(base, "missing", "z.yml"), nil, 0o644))
}

func TestReadFileRefusesALargeFile(t *testing.T) {
	defer func(limit int64) { maxReadBytes = limit }(maxReadBytes)
	maxReadBytes = 4
	repo := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(repo, "small"), []byte("1234"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(repo, "large"), []byte("12345"), 0o600))

	data, err := ReadFile(repo, filepath.Join(repo, "small"))
	require.NoError(t, err)
	assert.Equal(t, "1234", string(data))
	_, err = ReadFile(repo, filepath.Join(repo, "large"))
	assert.ErrorContains(t, err, "larger than 4 bytes")
	_, err = ReadFile(repo, filepath.Join(repo, "missing"))
	assert.Error(t, err)
	_, err = ReadFile(repo, filepath.Join(t.TempDir(), "elsewhere"))
	assert.Error(t, err)
}

func TestExistsStaysInTheRepository(t *testing.T) {
	base := t.TempDir()
	repo, outside := filepath.Join(base, "repo"), filepath.Join(base, "outside")
	require.NoError(t, os.MkdirAll(repo, 0o755))
	require.NoError(t, os.MkdirAll(outside, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(outside, "secret"), nil, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(repo, "here"), nil, 0o600))
	require.NoError(t, os.Symlink(outside, filepath.Join(repo, "link")))

	assert.True(t, Exists(repo, filepath.Join(repo, "here")))
	assert.True(t, Exists(repo, filepath.Join(repo, "link")), "the link itself is in the repository")
	assert.False(t, Exists(repo, filepath.Join(repo, "link", "secret")), "what it points at is not")
	assert.False(t, Exists(repo, filepath.Join(base, "outside", "secret")))
}
