package parser_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/MustacheCase/zanadir/models"
	"github.com/MustacheCase/zanadir/parser"
	"github.com/stretchr/testify/assert"
)

const testDir = "test-utils"

func setupGithubTestDir() error {
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

func TestGithubExists(t *testing.T) {
	err := setupGithubTestDir()
	assert.NoError(t, err)

	defer teardownTestDir()

	gp := parser.NewGithubParser()
	assert.True(t, gp.Exists(testDir))
	assert.False(t, gp.Exists("nonexistent-dir"))
}

func TestGithubParse(t *testing.T) {
	err := setupGithubTestDir()
	assert.NoError(t, err)

	defer teardownTestDir()

	gp := parser.NewGithubParser()
	artifacts, err := gp.Parse(testDir)
	assert.NoError(t, err)
	assert.Len(t, artifacts, 1)
	assert.Equal(t, "Test Workflow", artifacts[0].Name)
	assert.Len(t, artifacts[0].Jobs, 4)
	expectedJobs := map[string]string{
		"actions/cache":                         "v3",
		"actions/setup-node":                    "v18",
		"aws-actions/configure-aws-credentials": "v2",
		"./.github/workflows/deploy.yml":        "",
	}

	for _, job := range artifacts[0].Jobs {
		assert.Equal(t, expectedJobs[job.Package], job.Version, "Job version mismatch")
	}
}

// The parser previously skipped any step without a `uses:` key.
func TestGithubParserCapturesRunSteps(t *testing.T) {
	dir := t.TempDir()
	workflow := `
name: ci
jobs:
  security:
    steps:
      - uses: actions/checkout@v4
      - name: Scan
        run: trivy fs --exit-code 1 .
`
	err := os.WriteFile(filepath.Join(dir, "ci.yml"), []byte(workflow), 0600)
	assert.NoError(t, err)

	artifacts, err := parser.NewGithubParser().Parse(dir)
	assert.NoError(t, err)
	assert.Len(t, artifacts, 1)

	var runs, packages []string
	for _, job := range artifacts[0].Jobs {
		if job.Run != "" {
			runs = append(runs, job.Run)
		}
		if job.Package != "" {
			packages = append(packages, job.Package)
		}
	}
	assert.Equal(t, []string{"actions/checkout"}, packages)
	assert.Len(t, runs, 1)
	assert.Contains(t, runs[0], "trivy fs")
}

// parseOne writes a single workflow and returns the artifact parsed from it.
func parseOne(t *testing.T, workflow string) *models.Artifact {
	t.Helper()
	dir := t.TempDir()
	assert.NoError(t, os.WriteFile(filepath.Join(dir, "wf.yml"), []byte(workflow), 0o644))

	artifacts, err := parser.NewGithubParser().Parse(dir)
	assert.NoError(t, err)
	assert.Len(t, artifacts, 1)
	return artifacts[0]
}

func TestParseTriggerShapes(t *testing.T) {
	tests := []struct {
		name     string
		on       string
		expected []models.Trigger
	}{
		{
			name:     "a single event",
			on:       "on: push",
			expected: []models.Trigger{{Event: "push"}},
		},
		{
			name:     "a list of events",
			on:       "on: [push, pull_request]",
			expected: []models.Trigger{{Event: "push"}, {Event: "pull_request"}},
		},
		{
			name:     "a map with a branch filter",
			on:       "on:\n  push:\n    branches: [main, release]\n  pull_request:",
			expected: []models.Trigger{{Event: "push", Branches: []string{"main", "release"}}, {Event: "pull_request"}},
		},
		{
			name:     "a branch filter written as a scalar",
			on:       "on:\n  push:\n    branches: main",
			expected: []models.Trigger{{Event: "push", Branches: []string{"main"}}},
		},
		{
			// schedule's value is a list of crons, not a mapping of filters.
			name:     "an event whose value is not a mapping",
			on:       "on:\n  schedule:\n    - cron: '0 0 * * *'",
			expected: []models.Trigger{{Event: "schedule"}},
		},
		{
			name:     "no triggers at all",
			on:       "",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			artifact := parseOne(t, "name: w\n"+tt.on+"\njobs:\n  a:\n    steps:\n      - run: echo hi\n")
			assert.Equal(t, tt.expected, artifact.Triggers)
		})
	}
}

// A workflow that cannot fail the build is the signal the whole feature is
// for, and it can be set at either level.
func TestParseContinueOnError(t *testing.T) {
	stepLevel := parseOne(t, `
name: w
on: push
jobs:
  a:
    steps:
      - run: gitleaks detect
        continue-on-error: true
`)
	assert.True(t, stepLevel.Jobs[0].ContinueOnError)

	jobLevel := parseOne(t, `
name: w
on: push
jobs:
  a:
    continue-on-error: true
    steps:
      - run: gitleaks detect
`)
	assert.True(t, jobLevel.Jobs[0].ContinueOnError)

	neither := parseOne(t, `
name: w
on: push
jobs:
  a:
    steps:
      - run: gitleaks detect
`)
	assert.False(t, neither.Jobs[0].ContinueOnError)
}

// Both conditions gate the step and they are separate expressions, so merging
// them would lose which one is responsible.
func TestParseConditionsStaySeparate(t *testing.T) {
	artifact := parseOne(t, `
name: w
on: push
jobs:
  a:
    if: github.ref == 'refs/heads/main'
    steps:
      - run: gitleaks detect
        if: matrix.os == 'ubuntu-latest'
`)
	assert.Equal(t, "matrix.os == 'ubuntu-latest'", artifact.Jobs[0].If)
	assert.Equal(t, "github.ref == 'refs/heads/main'", artifact.Jobs[0].JobIf)
}

func TestParseNeedsAcceptsScalarOrList(t *testing.T) {
	scalar := parseOne(t, "name: w\non: push\njobs:\n  a:\n    needs: build\n    steps:\n      - run: x\n")
	assert.Equal(t, []string{"build"}, scalar.Jobs[0].Needs)

	list := parseOne(t, "name: w\non: push\njobs:\n  a:\n    needs: [build, test]\n    steps:\n      - run: x\n")
	assert.Equal(t, []string{"build", "test"}, list.Jobs[0].Needs)
}

// Action inputs are written as booleans and numbers as often as strings, and
// every consumer wants to match text.
func TestParseWithFlattensToStrings(t *testing.T) {
	artifact := parseOne(t, `
name: w
on: push
jobs:
  a:
    steps:
      - uses: aquasecurity/tfsec-action@v1.0.0
        with:
          soft_fail: true
          additional_args: --exclude-downloaded-modules
          retries: 3
`)
	assert.Equal(t, map[string]string{
		"soft_fail":       "true",
		"additional_args": "--exclude-downloaded-modules",
		"retries":         "3",
	}, artifact.Jobs[0].With)

	none := parseOne(t, "name: w\non: push\njobs:\n  a:\n    steps:\n      - run: x\n")
	assert.Nil(t, none.Jobs[0].With)
}

func TestHasTrigger(t *testing.T) {
	artifact := parseOne(t, "name: w\non: [push, schedule]\njobs:\n  a:\n    steps:\n      - run: x\n")
	assert.True(t, artifact.HasTrigger("push"))
	assert.True(t, artifact.HasTrigger("schedule"))
	assert.False(t, artifact.HasTrigger("pull_request"))
}
