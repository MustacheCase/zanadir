package parser_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/MustacheCase/zanadir/parser"
	"github.com/stretchr/testify/assert"
)

func setupGitlabTestDir() error {
	// Ensure test directory exists
	if err := os.MkdirAll(testDir, 0755); err != nil {
		return err
	}

	// Create a mock GitLab CI file
	gitlabCIContent := `
    stages:
      - test
    build:
      stage: test
      script:
        - echo "Building..."
      image: golang:1.19
    deploy:
      stage: test
      script:
        - echo "Deploying..."
      image: node:18
    `
	testFile := filepath.Join(testDir, ".gitlab-ci.yml")
	return os.WriteFile(testFile, []byte(gitlabCIContent), 0644)
}

func TestGitlabExists(t *testing.T) {
	err := setupGitlabTestDir()
	assert.NoError(t, err)

	defer teardownTestDir()

	gp := parser.NewGitlabParser()
	assert.True(t, gp.Exists(testDir))
	assert.False(t, gp.Exists("nonexistent-dir"))
}

func TesGitlabParse(t *testing.T) {
	err := setupGitlabTestDir()
	assert.NoError(t, err)

	defer teardownTestDir()

	gp := parser.NewGitlabParser()
	artifacts, err := gp.Parse(testDir)
	assert.NoError(t, err)
	assert.Len(t, artifacts, 1)
	assert.Equal(t, "GitLab CI/CD", artifacts[0].Name)
	assert.Len(t, artifacts[0].Jobs, 2)
	expectedJobs := map[string]string{
		"golang:1.19": "",
		"node:18":     "",
	}

	for _, job := range artifacts[0].Jobs {
		assert.Equal(t, expectedJobs[job.Package], job.Version, "Job version mismatch")
	}
}

func TestGitlabMalformedFile(t *testing.T) {
	err := setupGitlabTestDir()
	assert.NoError(t, err)

	defer teardownTestDir()

	// Overwrite the .gitlab-ci.yml file with malformed content
	testFile := filepath.Join(testDir, ".gitlab-ci.yml")
	malformedContent := `
    stages:
      - test
    build:
      stage: test
      script:
        - echo "Building..."
      image: golang:1.19
    deploy:
      stage: test
      script:
        - echo "Deploying..."
      image: node:18
      invalid_field: [`
	err = os.WriteFile(testFile, []byte(malformedContent), 0644)
	assert.NoError(t, err)

	gp := parser.NewGitlabParser()
	artifacts, err := gp.Parse(testDir)
	assert.Error(t, err, "Expected an error for a malformed file")
	assert.Nil(t, artifacts, "Expected no artifacts for a malformed file")
}

func TestGitlabParserHandlesNoStages(t *testing.T) {
	err := setupGitlabTestDir()
	assert.NoError(t, err)

	defer teardownTestDir()

	// Overwrite the .gitlab-ci.yml file with no stages
	testFile := filepath.Join(testDir, ".gitlab-ci.yml")
	noStagesContent := `
    build:
      script:
        - echo "Building..."
    `
	err = os.WriteFile(testFile, []byte(noStagesContent), 0644)
	assert.NoError(t, err)

	gp := parser.NewGitlabParser()
	artifacts, err := gp.Parse(testDir)
	assert.NoError(t, err)
	assert.Len(t, artifacts, 1, "Expected one artifact even with no stages")
	assert.Len(t, artifacts[0].Jobs, 1, "Expected one job even with no stages")
}

func TestGitlabParserHandlesDuplicateJobs(t *testing.T) {
	err := setupGitlabTestDir()
	assert.NoError(t, err)

	defer teardownTestDir()

	// Overwrite the .gitlab-ci.yml file with duplicate jobs
	testFile := filepath.Join(testDir, ".gitlab-ci.yml")
	duplicateJobsContent := `
    stages:
      - test
    build:
      stage: test
      script:
        - echo "Building..."
      image: golang:1.19
    build:
      stage: test
      script:
        - echo "Building again..."
      image: golang:1.19
    `
	err = os.WriteFile(testFile, []byte(duplicateJobsContent), 0644)
	assert.NoError(t, err)

	gp := parser.NewGitlabParser()
	artifacts, err := gp.Parse(testDir)
	assert.NoError(t, err)
	assert.Len(t, artifacts, 1, "Expected one artifact for duplicate jobs")
	assert.Len(t, artifacts[0].Jobs, 1, "Expected duplicate jobs to be merged")
}

func TestGitlabParserHandlesInvalidJobStructure(t *testing.T) {
	err := setupGitlabTestDir()
	assert.NoError(t, err)

	defer teardownTestDir()

	// Overwrite the .gitlab-ci.yml file with an invalid job structure
	testFile := filepath.Join(testDir, ".gitlab-ci.yml")
	invalidJobContent := `
    stages:
      - test
    build:
      stage: test
      invalid_field: true
    `
	err = os.WriteFile(testFile, []byte(invalidJobContent), 0644)
	assert.NoError(t, err)

	gp := parser.NewGitlabParser()
	artifacts, err := gp.Parse(testDir)
	assert.Error(t, err, "Expected an error for invalid job structure")
	assert.Nil(t, artifacts, "Expected no artifacts for invalid job structure")
}
