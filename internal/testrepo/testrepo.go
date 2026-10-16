// Package testrepo builds throwaway repositories for tests.
package testrepo

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// Write creates a temporary repository holding files, keyed by their path
// relative to its root, and returns the root. It is removed after the test.
func Write(t testing.TB, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for rel, content := range files {
		path := filepath.Join(dir, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	}
	return dir
}
