package lockfile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeLock(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, UV), []byte(content), 0o600))
	return dir
}

// Only git and URL sources are unauditable: a registry off pypi.org is often
// a mirror of it, and path-like sources are first-party code.
func TestUnauditableFlagsGitAndURLSources(t *testing.T) {
	dir := writeLock(t, `version = 1
requires-python = ">=3.12"

[[package]]
name = "en-core-web-sm"
version = "3.8.0"
source = { url = "https://github.com/explosion/spacy-models/releases/download/en_core_web_sm-3.8.0/en_core_web_sm-3.8.0-py3-none-any.whl" }
wheels = [
    { url = "https://github.com/explosion/spacy-models/releases/download/en_core_web_sm-3.8.0/en_core_web_sm-3.8.0-py3-none-any.whl", hash = "sha256:00" },
]

[[package]]
name = "forked"
version = "0.1.0"
source = { git = "https://github.com/example/forked?rev=main#0123456789abcdef" }

[[package]]
name = "requests"
version = "2.32.0"
source = { registry = "https://pypi.org/simple" }
sdist = { url = "https://files.pythonhosted.org/requests-2.32.0.tar.gz", hash = "sha256:00" }

[[package]]
name = "torch"
version = "2.13.0+cpu"
source = { registry = "https://download.pytorch.org/whl/cpu" }

[[package]]
name = "internal"
version = "1.0.0"
source = { registry = "https://artifactory.example.com/api/pypi/pypi/simple" }

[[package]]
name = "app"
version = "0.1.0"
source = { editable = "." }

[package.metadata]
requires-dist = [
    { name = "en-core-web-sm", url = "https://github.com/explosion/spacy-models/releases/download/en_core_web_sm-3.8.0/en_core_web_sm-3.8.0-py3-none-any.whl" },
]

[[package]]
name = "shared"
version = "0.1.0"
source = { path = "libs/shared" }

[[package]]
name = "member"
version = "0.1.0"
source = { virtual = "packages/member" }
`)
	assert.Equal(t, []string{"en-core-web-sm", "forked"}, Unauditable(dir))
}

// A package locked twice, once per platform, is named once.
func TestUnauditableNamesEachPackageOnce(t *testing.T) {
	dir := writeLock(t, `[[package]]
name = "forked"
version = "0.1.0"
source = { git = "https://github.com/example/forked?rev=a#a" }

[[package]]
name = "forked"
version = "0.2.0"
source = { git = "https://github.com/example/forked?rev=b#b" }
`)
	assert.Equal(t, []string{"forked"}, Unauditable(dir))
}

func TestUnauditableWithoutALockfile(t *testing.T) {
	assert.Empty(t, Unauditable(t.TempDir()))
}

// A remote scan leaves a lockfile it did not download as an empty placeholder.
func TestUnauditableOnAnEmptyPlaceholder(t *testing.T) {
	assert.Empty(t, Unauditable(writeLock(t, "")))
}
