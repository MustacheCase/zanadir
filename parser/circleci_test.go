package parser_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/MustacheCase/zanadir/parser"
	"github.com/stretchr/testify/assert"
)

const circleCITestDir = "test-circleci-utils"

func setupCircleCiTestDir() error {
	// Ensure test directory exists
	if err := os.MkdirAll(circleCITestDir, 0755); err != nil {
		return err
	}

	// Create a mock CircleCI workflow file
	workflowContent := `
version: 2.1
orbs:
  checkout: circleci/checkout@v4
  golang: circleci/golang@1.11
  codecov: circleci/codecov@3.2.4

jobs:
  test-and-build:
    docker:
      - image: cimg/go:1.20 # Use CircleCI Go image
    steps:
      # Checkout code using CircleCI's checkout orb
      - checkout/checkout

      # Set up Go environment using the golang orb
      - golang/install:
          version: "1.24"

      # Upload coverage reports to Codecov using the Codecov orb
      - codecov/upload:
          file: coverage.txt
          token: "${CODECOV_TOKEN}"

workflows:
  version: 2
  test-and-build:
    jobs:
      - test-and-build
`
	testFile := filepath.Join(circleCITestDir, "config.yml")
	return os.WriteFile(testFile, []byte(workflowContent), 0644)
}

func teardownCircleCiTestDir() {
	_ = os.RemoveAll(circleCITestDir)
}

func TestCircleCIExists(t *testing.T) {
	err := setupCircleCiTestDir()
	assert.NoError(t, err)

	defer teardownCircleCiTestDir()

	cp := parser.NewCircleCIParser()
	assert.True(t, cp.Exists(circleCITestDir))
	assert.False(t, cp.Exists("nonexistent-dir"))
}

func TestCircleCIParse(t *testing.T) {
	err := setupCircleCiTestDir()
	assert.NoError(t, err)

	defer teardownCircleCiTestDir()

	cp := parser.NewCircleCIParser()
	artifacts, err := cp.Parse(circleCITestDir)
	assert.NoError(t, err)
	assert.Len(t, artifacts, 1)
	assert.Equal(t, "CircleCI Workflow Orbs", artifacts[0].Name)
	assert.Len(t, artifacts[0].Jobs, 3)
	expectedJobs := map[string]string{
		"circleci/checkout": "v4",
		"circleci/golang":   "1.11",
		"circleci/codecov":  "3.2.4",
	}

	for _, job := range artifacts[0].Jobs {
		assert.Equal(t, expectedJobs[job.Package], job.Version, "Job version mismatch")
	}
}
