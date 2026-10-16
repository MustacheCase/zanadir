package parser_test

import (
	"testing"

	"github.com/MustacheCase/zanadir/internal/testrepo"
	"github.com/MustacheCase/zanadir/models"
	"github.com/MustacheCase/zanadir/parser"
	"github.com/stretchr/testify/assert"
)

const testGitlabConfig = `
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

func gitlabDir(t *testing.T, config string) string {
	t.Helper()
	return testrepo.Write(t, map[string]string{".gitlab-ci.yml": config})
}

func TestGitlabExists(t *testing.T) {
	gp := parser.NewGitlabParser()
	assert.True(t, gp.Exists(gitlabDir(t, testGitlabConfig)))
	assert.False(t, gp.Exists("nonexistent-dir"))
}

func TestGitlabParse(t *testing.T) {
	artifacts, err := parser.NewGitlabParser().Parse(gitlabDir(t, testGitlabConfig))
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

	artifacts, err := parser.NewGitlabParser().Parse(gitlabDir(t, malformedContent))
	assert.Error(t, err, "Expected an error for a malformed file")
	assert.Nil(t, artifacts, "Expected no artifacts for a malformed file")
}

func TestGitlabParserHandlesNoStages(t *testing.T) {
	noStagesContent := `
    build:
      script:
        - echo "Building..."
    `

	artifacts, err := parser.NewGitlabParser().Parse(gitlabDir(t, noStagesContent))
	assert.NoError(t, err)
	assert.Len(t, artifacts, 1, "Expected one artifact even with no stages")
	assert.Len(t, artifacts[0].Jobs, 1, "Expected one job even with no stages")
}

func TestGitlabParserHandlesDuplicateJobs(t *testing.T) {
	duplicateJobsContent := `
    stages:
      - test
    build1:
      stage: test
      script:
        - echo "Building..."
      image: golang:1.19
    build2:
      stage: test
      script:
        - echo "Building..."
      image: golang:1.19
    `

	artifacts, err := parser.NewGitlabParser().Parse(gitlabDir(t, duplicateJobsContent))
	assert.NoError(t, err)
	assert.Len(t, artifacts, 1, "Expected one artifact for duplicate jobs")
	assert.Len(t, artifacts[0].Jobs, 2, "Expected two jobs with identical definitions but unique names")
}

func TestGitlabParserHandlesInvalidJobStructure(t *testing.T) {
	invalidJobContent := `
    stages:
      - test
    build:
      stage: test
      invalid_field: true
    `

	artifacts, err := parser.NewGitlabParser().Parse(gitlabDir(t, invalidJobContent))

	// Adjust expectations based on the actual behavior of the Parse method
	assert.NoError(t, err, "Expected no error for invalid job structure")
	assert.Len(t, artifacts, 1, "Expected one artifact even with invalid job structure")
	assert.Len(t, artifacts[0].Jobs, 1, "Expected one job even with invalid job structure")
	assert.Equal(t, "build", artifacts[0].Jobs[0].Name, "Expected job name to be 'build'")
}

// The parser previously read only `image:` and ignored script blocks.
func TestGitlabParserCapturesScript(t *testing.T) {
	content := `
stages:
  - security
scan:
  stage: security
  image: alpine:3.20
  before_script:
    - apk add --no-cache curl
  script:
    - gitleaks detect --source .
`
	artifacts, err := parser.NewGitlabParser().Parse(gitlabDir(t, content))
	assert.NoError(t, err)
	assert.Len(t, artifacts, 1)
	assert.Len(t, artifacts[0].Jobs, 1)

	job := artifacts[0].Jobs[0]
	assert.Equal(t, "alpine:3.20", job.Package)
	assert.Contains(t, job.Run, "gitleaks detect")
	assert.Contains(t, job.Run, "apk add", "before_script should be captured too")
}

func gitlabRepo(t *testing.T, config string) *models.Artifact {
	t.Helper()
	artifacts, err := parser.NewGitlabParser().Parse(gitlabDir(t, config))
	assert.NoError(t, err)
	assert.Len(t, artifacts, 1)
	return artifacts[0]
}

// allow_failure is GitLab's continue-on-error, and when: manual is a job that
// only runs when somebody asks. Neither can stand between a change and the
// branch, and neither was read before.
func TestGitlabParsesWeakeningFields(t *testing.T) {
	tests := []struct {
		name                    string
		config                  string
		expectedContinueOnError bool
		expectedManual          bool
	}{
		{
			name:                    "allow_failure",
			config:                  "sast:\n  script:\n    - semgrep ci\n  allow_failure: true\n",
			expectedContinueOnError: true,
		},
		{
			name:   "allow_failure set to false",
			config: "sast:\n  script:\n    - semgrep ci\n  allow_failure: false\n",
		},
		{
			name:           "when manual",
			config:         "sast:\n  script:\n    - semgrep ci\n  when: manual\n",
			expectedManual: true,
		},
		{
			name:           "when manual in any case",
			config:         "sast:\n  script:\n    - semgrep ci\n  when: Manual\n",
			expectedManual: true,
		},
		{
			name:   "when on_success is the default and weakens nothing",
			config: "sast:\n  script:\n    - semgrep ci\n  when: on_success\n",
		},
		{
			// Runs automatically, just later.
			name:   "when delayed is not manual",
			config: "sast:\n  script:\n    - semgrep ci\n  when: delayed\n  start_in: 5 minutes\n",
		},
		{
			name:   "neither set",
			config: "sast:\n  script:\n    - semgrep ci\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			artifact := gitlabRepo(t, tt.config)
			assert.Len(t, artifact.Jobs, 1)
			assert.Equal(t, tt.expectedContinueOnError, artifact.Jobs[0].ContinueOnError)
			assert.Equal(t, tt.expectedManual, artifact.Jobs[0].Manual)
		})
	}
}
