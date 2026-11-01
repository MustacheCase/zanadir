// Package testrepo builds throwaway repositories for tests.
package testrepo

import (
	"os"
	"path/filepath"
	"runtime"
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

// Touch creates a temporary repository holding placeholder files at paths,
// for tests where a file's presence is all that matters.
func Touch(t testing.TB, paths ...string) string {
	t.Helper()
	files := make(map[string]string, len(paths))
	for _, path := range paths {
		files[path] = "x"
	}
	return Write(t, files)
}

// Root and Windows do not enforce permission bits.
var permissionsIgnored = os.Geteuid() == 0 || runtime.GOOS == "windows"

// NeedsPermissions skips a test that relies on a file or directory it cannot
// read or write.
func NeedsPermissions(t testing.TB) {
	t.Helper()
	if permissionsIgnored {
		t.Skip("permission bits are not enforced here")
	}
}
