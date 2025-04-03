package parser

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGitlabParser_Exists(t *testing.T) {
	tempDir := t.TempDir()
	gitlabFile := filepath.Join(tempDir, ".gitlab-ci.yml")
	os.WriteFile(gitlabFile, []byte("stages:\n  - test\n"), 0644)

	parser := NewGitlabParser()
	assert.True(t, parser.Exists(tempDir))
}

func TestGitlabParser_Parse(t *testing.T) {
	tempDir := t.TempDir()
	gitlabFile := filepath.Join(tempDir, ".gitlab-ci.yml")
	yamlContent := `
    stages:
      - test
    build:
      stage: test
      script:
        - echo "Building..."
      image: golang:1.19
    `
	os.WriteFile(gitlabFile, []byte(yamlContent), 0644)

	parser := NewGitlabParser()
	artifacts, err := parser.Parse(tempDir)

	assert.NoError(t, err)
	assert.Len(t, artifacts, 1)
	assert.Equal(t, "GitLab CI/CD", artifacts[0].Name)
	assert.Len(t, artifacts[0].Jobs, 1)
	assert.Equal(t, "build", artifacts[0].Jobs[0].Name)
	assert.Equal(t, "golang:1.19", artifacts[0].Jobs[0].Package)
}
