package parser_test

import (
	"testing"

	"github.com/MustacheCase/zanadir/internal/testrepo"
	"github.com/MustacheCase/zanadir/parser"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testCircleCIConfig = `
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

func circleCIDir(t *testing.T, config string) string {
	t.Helper()
	return testrepo.Write(t, map[string]string{".circleci/config.yml": config})
}

func TestCircleCIExists(t *testing.T) {
	cp := parser.NewCircleCIParser()
	assert.True(t, cp.Exists(circleCIDir(t, testCircleCIConfig)))
	assert.False(t, cp.Exists("nonexistent-dir"))
}

func TestCircleCIParse(t *testing.T) {
	artifacts, err := parser.NewCircleCIParser().Parse(circleCIDir(t, testCircleCIConfig))
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

// The parser declared a Steps field but never read it; only orbs were parsed.
func TestCircleCIParserCapturesRunSteps(t *testing.T) {
	config := `
version: 2.1
jobs:
  security:
    docker:
      - image: cimg/base:stable
    steps:
      - checkout
      - run: gitleaks detect --source .
      - run:
          name: SCA scan
          command: trivy fs .
`
	artifacts, err := parser.NewCircleCIParser().Parse(circleCIDir(t, config))
	assert.NoError(t, err)
	assert.Len(t, artifacts, 1)

	var run string
	for _, job := range artifacts[0].Jobs {
		if job.Run != "" {
			run = job.Run
		}
	}
	assert.Contains(t, run, "gitleaks detect", "plain run form")
	assert.Contains(t, run, "trivy fs", "run with a command key")
}

func TestCircleCISkipsUnparseableYAML(t *testing.T) {
	artifacts, err := parser.NewCircleCIParser().Parse(circleCIDir(t, "jobs: [oops\n"))
	require.NoError(t, err)
	require.Len(t, artifacts, 1)
	assert.NotEmpty(t, artifacts[0].Skipped)
	assert.Empty(t, artifacts[0].Jobs)
}

// CircleCI accepts a mapping that merges twice, which yaml.v3 alone rejects
// as a duplicate key.
func TestCircleCIReadsRepeatedMergeKeys(t *testing.T) {
	artifacts, err := parser.NewCircleCIParser().Parse(circleCIDir(t, `
references:
  env: &env
    environment:
      CGO_ENABLED: "0"
  build: &build
    steps:
      - run: gitleaks detect
jobs:
  "golang:1.17":
    <<: *env
    <<: *build
    docker:
      - image: cimg/go:1.17
  lint:
    <<: [*env]
    <<: *build
`))
	require.NoError(t, err)
	require.Len(t, artifacts, 1)
	assert.Empty(t, artifacts[0].Skipped)
	runs := map[string]string{}
	for _, job := range artifacts[0].Jobs {
		runs[job.Name] = job.Run
	}
	assert.Equal(t, map[string]string{"golang:1.17": "gitleaks detect", "lint": "gitleaks detect"}, runs)
}

// docker: entries carry nested maps and workflows: lists jobs as mappings;
// typed fields for either aborted the scan of real configurations.
func TestCircleCIToleratesShapesItDoesNotRead(t *testing.T) {
	artifacts, err := parser.NewCircleCIParser().Parse(circleCIDir(t, `
version: 2.1
orbs:
  python: circleci/python@2.1.1
  local:
    commands:
      hello:
        steps: [checkout]
jobs:
  build:
    docker:
      - image: cimg/python:3.10.2
        environment:
          LIMIT: "2"
        auth:
          username: $USER
    steps:
      - run: pytest
workflows:
  main:
    jobs:
      - build:
          filters: {branches: {only: main}}
`))
	assert.NoError(t, err)
	assert.Len(t, artifacts, 1)

	packages := map[string]string{}
	for _, job := range artifacts[0].Jobs {
		packages[job.Name] = job.Package + "|" + job.Version + "|" + job.Run
	}
	assert.Equal(t, map[string]string{"python": "circleci/python|2.1.1|", "build": "||pytest"}, packages)
}
