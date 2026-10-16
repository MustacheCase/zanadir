package scanner

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/MustacheCase/zanadir/models"
	"github.com/MustacheCase/zanadir/parser"
	"github.com/stretchr/testify/assert"
)

// fakeParser stands in for one platform. Every parser is handed the same
// repository root, so each position in the scanner gets its own fake.
type fakeParser struct {
	exists    bool
	artifacts []*models.Artifact
	err       error
	gotRoot   string
}

func (f *fakeParser) Exists(root string) bool { f.gotRoot = root; return f.exists }

func (f *fakeParser) Parse(string) ([]*models.Artifact, error) { return f.artifacts, f.err }

func artifactNamed(name string) []*models.Artifact { return []*models.Artifact{{Name: name}} }

func TestRepositoryScannerHandsEveryParserTheRoot(t *testing.T) {
	github, gitlab := &fakeParser{}, &fakeParser{exists: true, artifacts: artifactNamed("gitlab")}
	artifacts, err := (&RepositoryScanner{parsers: []parser.Parser{github, gitlab}}).Scan("/repo")

	assert.NoError(t, err)
	assert.Equal(t, artifactNamed("gitlab"), artifacts)
	assert.Equal(t, "/repo", github.gotRoot)
	assert.Equal(t, "/repo", gitlab.gotRoot)
}

// Scanning used to stop at the first parser that found anything, so a
// repository using two platforms had the second one go unread - and a
// dependency bot declared outside the pipeline could never be seen at all.
func TestRepositoryScannerReadsEveryParser(t *testing.T) {
	artifacts, err := (&RepositoryScanner{parsers: []parser.Parser{
		&fakeParser{exists: true, artifacts: artifactNamed("from-github")},
		&fakeParser{exists: true, artifacts: artifactNamed("from-circleci")},
	}}).Scan("/repo")

	assert.NoError(t, err)
	assert.Equal(t, append(artifactNamed("from-github"), artifactNamed("from-circleci")...), artifacts)
}

func TestRepositoryScannerWithNoCI(t *testing.T) {
	artifacts, err := (&RepositoryScanner{parsers: []parser.Parser{&fakeParser{}, &fakeParser{}}}).Scan("/repo")
	assert.NoError(t, err)
	assert.Nil(t, artifacts)
}

func TestRepositoryScannerStopsOnAParseError(t *testing.T) {
	artifacts, err := (&RepositoryScanner{parsers: []parser.Parser{
		&fakeParser{exists: true, artifacts: artifactNamed("read")},
		&fakeParser{exists: true, err: errors.New("parse error")},
	}}).Scan("/repo")

	assert.Error(t, err)
	assert.Nil(t, artifacts)
}

// The fakes above cannot catch a parser looking in the wrong place, which is
// how GitLab CI once went undetected. This exercises the real parsers.
func TestRepositoryScannerFindsEveryPlatform(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
	}{
		{
			name: "github actions",
			files: map[string]string{
				".github/workflows/ci.yml": "name: ci\non: pull_request\njobs:\n  a:\n    steps:\n      - run: semgrep ci\n",
			},
		},
		{
			name: "gitlab ci",
			files: map[string]string{
				".gitlab-ci.yml": "sast:\n  script:\n    - semgrep ci\n",
			},
		},
		{
			name: "circleci",
			files: map[string]string{
				".circleci/config.yml": "version: 2.1\njobs:\n  sast:\n    steps:\n      - run: semgrep ci\n",
			},
		},
		{
			name: "bitbucket pipelines",
			files: map[string]string{
				"bitbucket-pipelines.yml": "pipelines:\n  pull-requests:\n    '**':\n      - step:\n          script:\n            - semgrep ci\n",
			},
		},
		{
			name: "azure pipelines",
			files: map[string]string{
				"azure-pipelines.yml": "pr: [main]\nsteps:\n  - script: semgrep ci\n",
			},
		},
		{
			name: "dependabot",
			files: map[string]string{
				".github/dependabot.yml": "version: 2\n",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for rel, content := range tt.files {
				path := filepath.Join(dir, rel)
				assert.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
				assert.NoError(t, os.WriteFile(path, []byte(content), 0o644))
			}

			artifacts, err := NewRepositoryScanner().Scan(dir)

			assert.NoError(t, err)
			assert.NotEmpty(t, artifacts, "the platform's configuration was not read at all")
		})
	}
}
