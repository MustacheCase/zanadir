package parser_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/MustacheCase/zanadir/parser"
	"github.com/stretchr/testify/assert"
)

const gitlabTestDir = "test-gitlab-utils"

func setupGitLabTestDir() error {
	// Ensure test directory exists
	if err := os.MkdirAll(gitlabTestDir, 0755); err != nil {
		return err
	}

	// Create a mock GitLab CI/CD workflow file
	workflowContent := `
stages:
  - build
  - test

jobs:
  build-job:
    stage: build
    script:
      - echo "Building..."

  test-job:
    stage: test
    script:
      - echo "Testing..."
`
	testFile := filepath.Join(gitlabTestDir, ".gitlab-ci.yml")
	return os.WriteFile(testFile, []byte(workflowContent), 0644)
}

func teardownGitLabTestDir() {
	_ = os.RemoveAll(gitlabTestDir)
}

func TestGitLabExists(t *testing.T) {
	err := setupGitLabTestDir()
	assert.NoError(t, err)

	defer teardownGitLabTestDir()

	gp := parser.NewGitLabParser()
	assert.True(t, gp.Exists(gitlabTestDir))
	assert.False(t, gp.Exists("nonexistent-dir"))
}

func TestGitLabParse(t *testing.T) {
	err := setupGitLabTestDir()
	assert.NoError(t, err)

	defer teardownGitLabTestDir()

	gp := parser.NewGitLabParser()
	artifacts, err := gp.Parse(gitlabTestDir)
	assert.NoError(t, err)
	assert.Len(t, artifacts, 1)
	assert.Equal(t, "GitLab CI/CD Workflow", artifacts[0].Name)
	assert.Len(t, artifacts[0].Jobs, 2)
	expectedJobs := map[string]string{
		"build-job": "echo \"Building...\"",
		"test-job":  "echo \"Testing...\"",
	}

	for _, job := range artifacts[0].Jobs {
		assert.Equal(t, expectedJobs[job.Name], job.Version, "Job version mismatch")
	}
}
