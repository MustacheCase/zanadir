package scanner

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/MustacheCase/zanadir/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type MockParser struct {
	mock.Mock
}

func (m *MockParser) Exists(path string) bool {
	args := m.Called(path)
	return args.Bool(0)
}

func (m *MockParser) Parse(path string) ([]*models.Artifact, error) {
	args := m.Called(path)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.Artifact), args.Error(1)
}

var (
	mockParser *MockParser
)

func setup() {
	mockParser = new(MockParser)
}

func TestRepositoryScanner_Scan(t *testing.T) {
	setup()

	repoDir := "/test/repo"
	githubCIPath := filepath.Join(repoDir, "/.github/workflows/")
	circleCIPath := filepath.Join(repoDir, "/.circleci/")

	gitlabCIPath := filepath.Join(repoDir, "/.gitlab-ci.yml")

	mockParser.On("Exists", githubCIPath).Return(false)
	mockParser.On("Exists", circleCIPath).Return(true)
	mockParser.On("Parse", circleCIPath).Return([]*models.Artifact{{Name: "artifact1"}}, nil)
	mockParser.On("Exists", gitlabCIPath).Return(false)

	s := &RepositoryScanner{
		ciParsers: []ciParser{
			{Path: "/.github/workflows/", Parser: mockParser},
			{Path: "/.circleci/", Parser: mockParser},
			{Path: "/.gitlab-ci.yml", Parser: mockParser},
		},
	}
	artifacts, err := s.Scan(repoDir)

	assert.NoError(t, err)
	assert.NotNil(t, artifacts)
	assert.Len(t, artifacts, 1)
	assert.Equal(t, "artifact1", artifacts[0].Name)

	mockParser.AssertExpectations(t)
}

// Scanning used to stop at the first parser that found anything, so a
// repository using two platforms had the second one go unread - and a
// dependency bot declared outside the pipeline could never be seen at all.
func TestRepositoryScannerReadsEveryParser(t *testing.T) {
	setup()

	repoDir := "/test/repo"
	githubCIPath := filepath.Join(repoDir, "/.github/workflows/")
	circleCIPath := filepath.Join(repoDir, "/.circleci/")

	mockParser.On("Exists", githubCIPath).Return(true)
	mockParser.On("Parse", githubCIPath).Return([]*models.Artifact{{Name: "from-github"}}, nil)
	mockParser.On("Exists", circleCIPath).Return(true)
	mockParser.On("Parse", circleCIPath).Return([]*models.Artifact{{Name: "from-circleci"}}, nil)

	s := &RepositoryScanner{
		ciParsers: []ciParser{
			{Path: "/.github/workflows/", Parser: mockParser},
			{Path: "/.circleci/", Parser: mockParser},
		},
	}
	artifacts, err := s.Scan(repoDir)

	assert.NoError(t, err)
	assert.Len(t, artifacts, 2)
	assert.Equal(t, "from-github", artifacts[0].Name)
	assert.Equal(t, "from-circleci", artifacts[1].Name)

	mockParser.AssertExpectations(t)
}

func TestRepositoryScanner_Scan_NoCI(t *testing.T) {
	setup()

	repoDir := "/test/repo"
	githubCIPath := filepath.Join(repoDir, "/.github/workflows/")
	circleCIPath := filepath.Join(repoDir, "/.circleci/")
	gitlabCIPath := filepath.Join(repoDir, "/.gitlab-ci.yml")

	mockParser.On("Exists", githubCIPath).Return(false)
	mockParser.On("Exists", circleCIPath).Return(false)
	mockParser.On("Exists", gitlabCIPath).Return(false)

	s := &RepositoryScanner{
		ciParsers: []ciParser{
			{Path: "/.github/workflows/", Parser: mockParser},
			{Path: "/.circleci/", Parser: mockParser},
			{Path: "/.gitlab-ci.yml", Parser: mockParser},
		},
	}
	artifacts, err := s.Scan(repoDir)

	assert.NoError(t, err)
	assert.Nil(t, artifacts)

	mockParser.AssertExpectations(t)
}

func TestRepositoryScanner_Scan_ParseError(t *testing.T) {
	setup()

	repoDir := "/test/repo"
	githubCIPath := filepath.Join(repoDir, "/.github/workflows/")
	circleCIPath := filepath.Join(repoDir, "/.circleci/")

	mockParser.On("Exists", githubCIPath).Return(false)
	mockParser.On("Exists", circleCIPath).Return(true)
	mockParser.On("Parse", circleCIPath).Return(nil, errors.New("parse error"))
	mockParser.On("Exists", filepath.Join(repoDir, "/.gitlab-ci.yml")).Return(false).Maybe()

	s := &RepositoryScanner{
		ciParsers: []ciParser{
			{Path: "/.github/workflows/", Parser: mockParser},
			{Path: "/.circleci/", Parser: mockParser},
			{Path: "/.gitlab-ci.yml", Parser: mockParser},
		},
	}
	artifacts, err := s.Scan(repoDir)

	assert.Error(t, err)
	assert.Nil(t, artifacts)

	mockParser.AssertExpectations(t)
}

// The mocked tests above cannot catch a path-convention mismatch between the
// scanner and a parser, which is how GitLab CI went undetected: the scanner
// handed the parser the file path and the parser appended the filename again,
// so Exists never returned true. This exercises the real parsers.
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
