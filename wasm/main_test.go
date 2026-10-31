//go:build js && wasm

package main

import (
	"encoding/json"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/MustacheCase/zanadir/handler"
	"github.com/MustacheCase/zanadir/internal/repofs"
	"github.com/MustacheCase/zanadir/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These run on web/scan/memfs.js (see wasm/run.js), the file system the page
// gives Go, so they check it does what zanadir asks of it.

func TestFileSystem(t *testing.T) {
	dir, err := os.MkdirTemp("", "zanadir-")
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(dir, "/tmp/zanadir-"), dir)

	require.NoError(t, os.MkdirAll(filepath.Join(dir, "a", "b"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a", "b", "x.yml"), []byte("one\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a", "z.txt"), nil, 0o644))

	data, err := os.ReadFile(filepath.Join(dir, "a", "b", "x.yml"))
	require.NoError(t, err)
	assert.Equal(t, "one\n", string(data))

	f, err := os.OpenFile(filepath.Join(dir, "a", "b", "x.yml"), os.O_APPEND|os.O_WRONLY, 0)
	require.NoError(t, err)
	_, err = f.WriteString("two\n")
	require.NoError(t, err)
	require.NoError(t, f.Close())
	f, err = os.Open(filepath.Join(dir, "a", "b", "x.yml"))
	require.NoError(t, err)
	buf := make([]byte, 3)
	_, err = f.ReadAt(buf, 4)
	require.NoError(t, err)
	assert.Equal(t, "two", string(buf))
	_, err = f.Seek(0, io.SeekStart)
	require.NoError(t, err)
	all, err := io.ReadAll(f)
	require.NoError(t, err)
	assert.Equal(t, "one\ntwo\n", string(all))
	require.NoError(t, f.Close())

	entries, err := os.ReadDir(filepath.Join(dir, "a"))
	require.NoError(t, err)
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	assert.Equal(t, []string{"b", "z.txt"}, names)
	info, err := os.Stat(filepath.Join(dir, "a", "b"))
	require.NoError(t, err)
	assert.True(t, info.IsDir())

	var walked []string
	require.NoError(t, filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		rel, _ := filepath.Rel(dir, path)
		walked = append(walked, rel)
		return err
	}))
	assert.Equal(t, []string{".", "a", "a/b", "a/b/x.yml", "a/z.txt"}, walked)

	read, err := repofs.ReadFile(dir, filepath.Join(dir, "a", "b", "x.yml"))
	require.NoError(t, err, "os.Root works too")
	assert.Equal(t, "one\ntwo\n", string(read))
	assert.True(t, repofs.Exists(dir, filepath.Join(dir, "a", "z.txt")))
	assert.False(t, repofs.Exists(dir, filepath.Join(dir, "missing")))

	require.NoError(t, os.Rename(filepath.Join(dir, "a", "z.txt"), filepath.Join(dir, "z.txt")))
	assert.FileExists(t, filepath.Join(dir, "z.txt"))
	require.NoError(t, os.Truncate(filepath.Join(dir, "a", "b", "x.yml"), 2))
	data, _ = os.ReadFile(filepath.Join(dir, "a", "b", "x.yml"))
	assert.Equal(t, "on", string(data))

	_, err = os.ReadFile(filepath.Join(dir, "missing"))
	assert.ErrorIs(t, err, fs.ErrNotExist)
	assert.ErrorIs(t, os.Mkdir(filepath.Join(dir, "a"), 0o755), fs.ErrExist)
	assert.Error(t, os.Remove(filepath.Join(dir, "a")), "not empty")

	require.NoError(t, os.RemoveAll(dir))
	_, err = os.Stat(dir)
	assert.ErrorIs(t, err, fs.ErrNotExist)
}

func TestScanOnTheInMemoryFileSystem(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, ".git"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".github", "workflows"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".github", "workflows", "ci.yml"), []byte(`on: pull_request
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - run: go test ./...
      - run: gitleaks detect || true
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), nil, 0o644))

	h, err := handler.Setup()
	require.NoError(t, err)
	report, err := scanDir(h, dir, "json")
	require.NoError(t, err)

	var parsed struct {
		Uncovered []struct{ ID string }
		Weakened  []struct{ Category, Verdict string }
	}
	require.NoError(t, json.Unmarshal([]byte(report), &parsed))
	assert.Contains(t, parsed.Weakened, struct{ Category, Verdict string }{string(models.Secrets), "advisory"}, "|| true is read")
	assert.False(t, slices.ContainsFunc(parsed.Uncovered, func(c struct{ ID string }) bool { return c.ID == string(models.UnitTests) }), "go test is found")
}

func TestRunRefusesWhatIsNotARepository(t *testing.T) {
	_, err := run("not a repository", "json")
	assert.ErrorContains(t, err, "not a repository")
}
