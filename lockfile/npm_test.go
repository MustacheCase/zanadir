package lockfile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeNPMLock(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, NPM), []byte(content), 0o600))
	return dir
}

// Git and plain tarball URLs are unauditable; a registry tarball is not,
// whether npm's, a mirror's or GitHub Packages', and nor is first-party code.
func TestUnauditableNPMFlagsGitAndTarballSources(t *testing.T) {
	dir := writeNPMLock(t, `{
  "name": "app",
  "lockfileVersion": 3,
  "packages": {
    "": {"name": "app", "dependencies": {"forked": "github:example/forked"}},
    "node_modules/forked": {"version": "1.0.0", "resolved": "git+ssh://git@github.com/example/forked.git#0123456789abcdef"},
    "node_modules/@scope/vendored": {"version": "2.0.0", "resolved": "https://codeload.github.com/example/vendored/tar.gz/0123456"},
    "node_modules/lodash": {"version": "4.17.21", "resolved": "https://registry.npmjs.org/lodash/-/lodash-4.17.21.tgz"},
    "node_modules/internal": {"version": "1.0.0", "resolved": "https://artifactory.example.com/api/npm/npm/internal/-/internal-1.0.0.tgz"},
    "node_modules/@org/private": {"version": "1.0.0", "resolved": "https://npm.pkg.github.com/download/@org/private/1.0.0/abcdef"},
    "node_modules/a/node_modules/forked": {"version": "1.0.0", "resolved": "git+https://github.com/example/forked.git#0123456789abcdef"},
    "node_modules/local": {"resolved": "packages/local", "link": true},
    "node_modules/tool": {"version": "0.1.0", "resolved": "file:../tool"},
    "packages/local": {"name": "local", "version": "0.1.0"}
  }
}`)
	assert.Equal(t, []string{"@scope/vendored", "forked"}, UnauditableNPM(dir))
}

// Lockfile v1 has only the nested dependencies tree, where a git source can
// sit in version.
func TestUnauditableNPMReadsLockfileV1(t *testing.T) {
	dir := writeNPMLock(t, `{
  "lockfileVersion": 1,
  "dependencies": {
    "lodash": {"version": "4.17.21", "resolved": "https://registry.npmjs.org/lodash/-/lodash-4.17.21.tgz"},
    "parent": {
      "version": "1.0.0",
      "resolved": "https://registry.npmjs.org/parent/-/parent-1.0.0.tgz",
      "dependencies": {"forked": {"version": "github:example/forked#0123456"}}
    }
  }
}`)
	assert.Equal(t, []string{"forked"}, UnauditableNPM(dir))
}

func TestUnauditableNPMWithoutAReadableLockfile(t *testing.T) {
	assert.Empty(t, UnauditableNPM(t.TempDir()))
	assert.Empty(t, UnauditableNPM(writeNPMLock(t, "not json")))
}
