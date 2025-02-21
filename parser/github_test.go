package parser_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/MustacheCase/zanadir/parser"
	"github.com/stretchr/testify/assert"
)

const testDir = "test-utils"

func setupTestDir() error {
	// Ensure test directory exists
	if err := os.MkdirAll(testDir, 0755); err != nil {
		return err
	}

	// Create a mock workflow file
	workflowContent := `
name: Test Workflow
jobs:
  build:
    steps:
      - name: Cache
        uses: actions/cache@v3
      - name: Setup Node
        uses: actions/setup-node@v18
  deploy:
    steps:
      - name: Deploy to AWS
        uses: aws-actions/configure-aws-credentials@v2
      - name: Deploy
        uses: ./.github/workflows/deploy.yml
`
	testFile := filepath.Join(testDir, "test-workflow.yml")
	return os.WriteFile(testFile, []byte(workflowContent), 0644)
}

func teardownTestDir() {
	_ = os.RemoveAll(testDir)
}

func TestExists(t *testing.T) {
	err := setupTestDir()
	assert.NoError(t, err)

	defer teardownTestDir()

	gp := parser.NewGithubParser()
	assert.True(t, gp.Exists(testDir))
	assert.False(t, gp.Exists("nonexistent-dir"))
}

func TestParse(t *testing.T) {
	err := setupTestDir()
	assert.NoError(t, err)

	defer teardownTestDir()

	gp := parser.NewGithubParser()
	artifacts, err := gp.Parse(testDir)
	assert.NoError(t, err)
	assert.Len(t, artifacts, 1)
	assert.Equal(t, "Test Workflow", artifacts[0].Name)
	assert.Len(t, artifacts[0].Jobs, 4)
	assert.Equal(t, "actions/cache", artifacts[0].Jobs[0].Name)
	assert.Equal(t, "v3", artifacts[0].Jobs[0].Version)
	assert.Equal(t, "aws-actions/configure-aws-credentials", artifacts[0].Jobs[2].Name)
	assert.Equal(t, "v2", artifacts[0].Jobs[2].Version)
}
