package parser

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/MustacheCase/zanadir/internal/testrepo"
	"github.com/MustacheCase/zanadir/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func parseAzure(t *testing.T, files map[string]string) *models.Artifact {
	t.Helper()
	artifacts, err := NewAzureParser().Parse(testrepo.Write(t, files))
	require.NoError(t, err)
	require.Len(t, artifacts, 1)
	return artifacts[0]
}

func TestAzureTriggers(t *testing.T) {
	tests := []struct {
		name     string
		head     string
		expected []models.Trigger
	}{
		{"pr branches listed", "pr: [main]", []models.Trigger{{Event: "pull_request"}}},
		{"pr as a mapping", "pr:\n  branches:\n    include: ['*']", []models.Trigger{{Event: "pull_request"}}},
		{"pr left out is a branch policy's call", "trigger: [main]", nil},
		{"pr none with a push trigger", "trigger: [main]\npr: none", nil},
		{"schedule only", "trigger: none\npr: none\nschedules:\n  - cron: '0 0 * * *'\n    branches: {include: [main]}", []models.Trigger{{Event: "schedule"}}},
		{"schedule with pr left out", "trigger: none\nschedules:\n  - cron: '0 0 * * *'", nil},
		{"empty pr list", "pr: []", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			artifact := parseAzure(t, map[string]string{"azure-pipelines.yml": tt.head + "\nsteps:\n  - script: semgrep ci\n"})
			assert.Equal(t, tt.expected, artifact.Triggers)
		})
	}
}

func TestAzureStepsJobsAndStages(t *testing.T) {
	artifact := parseAzure(t, map[string]string{"azure-pipelines.yaml": `
pr: [main]
stages:
  - stage: Build
    condition: succeeded()
    jobs:
      - job: Scan
        continueOnError: true
        steps:
          - checkout: self
          - task: SnykSecurityScan@1
            inputs:
              failOnIssues: false
              severityThreshold: high
              list: [a, b]
      - job: Lint
        condition: eq(variables['Build.Reason'], 'PullRequest')
        steps:
          - bash: npx eslint .
            condition: succeeded()
          - pwsh: Invoke-Pester
            condition: always()
          - powershell: Write-Host hi
            enabled: false
          - script: gitleaks detect
            continueOnError: true
  - stage: Deploy
    condition: ne(variables['Build.Reason'], 'PullRequest')
    jobs:
      - deployment: Release
        strategy:
          runOnce:
            deploy:
              steps:
                - script: cosign sign $(image)
            on:
              failure:
                steps:
                  - script: ./rollback.sh
`})

	assert.Equal(t, "Azure Pipelines", artifact.Name)
	assert.Equal(t, "azure-pipelines.yaml", filepath.Base(artifact.Location))
	require.Len(t, artifact.Jobs, 6)

	snyk := artifact.Jobs[0]
	assert.Equal(t, "Scan", snyk.Name)
	assert.Equal(t, "SnykSecurityScan", snyk.Package)
	assert.Equal(t, "1", snyk.Version)
	assert.Equal(t, map[string]string{"failOnIssues": "false", "severityThreshold": "high"}, snyk.With)
	assert.True(t, snyk.ContinueOnError, "a job's continueOnError covers its steps")
	assert.Empty(t, snyk.JobIf, "succeeded() is the default and says nothing")

	eslint := artifact.Jobs[1]
	assert.Equal(t, "npx eslint .", eslint.Run)
	assert.Empty(t, eslint.If)
	assert.Equal(t, "eq(variables['Build.Reason'], 'PullRequest')", eslint.JobIf)
	assert.False(t, eslint.ContinueOnError)

	assert.Equal(t, "Invoke-Pester", artifact.Jobs[2].Run)
	assert.Equal(t, "always()", artifact.Jobs[2].If)

	gitleaks := artifact.Jobs[3]
	assert.Equal(t, "gitleaks detect", gitleaks.Run, "the disabled powershell step is not there")
	assert.True(t, gitleaks.ContinueOnError)

	cosign := artifact.Jobs[4]
	assert.Equal(t, "Release", cosign.Name)
	assert.Equal(t, "cosign sign $(image)", cosign.Run)
	assert.Equal(t, "ne(variables['Build.Reason'], 'PullRequest')", cosign.JobIf)
	assert.Equal(t, "./rollback.sh", artifact.Jobs[5].Run)
}

func TestAzureConditionsNest(t *testing.T) {
	artifact := parseAzure(t, map[string]string{"azure-pipelines.yml": `
stages:
  - stage: A
    condition: eq(1, 1)
    jobs:
      - job: B
        condition: eq(2, 2)
        steps:
          - script: semgrep ci
`})
	assert.Equal(t, "eq(1, 1) && eq(2, 2)", artifact.Jobs[0].JobIf)
}

func TestAzureFollowsLocalTemplates(t *testing.T) {
	artifact := parseAzure(t, map[string]string{
		"azure-pipelines.yml": `
pr: [main]
extends:
  template: ci/extends.yml
`,
		"ci/extends.yml": `
stages:
  - template: stages.yml
`,
		"ci/stages.yml": `
stages:
  - stage: Build
    jobs:
      - template: /ci/jobs.yml@self
      - template: jobs.yml@other-repo
      - template: ${{ parameters.jobs }}
      - template: ../../outside.yml
      - template: missing.yml
`,
		"ci/jobs.yml": `
jobs:
  - job: Test
    continueOnError: true
    steps:
      - template: steps/scan.yml
`,
		"ci/steps/scan.yml": `
parameters:
  - name: x
steps:
  - script: trivy fs .
  - template: scan.yml
`,
	})

	// scan.yml includes itself, so it is read once per level until the depth
	// bound stops it, each copy carrying the job it was reached from.
	require.NotEmpty(t, artifact.Jobs)
	assert.LessOrEqual(t, len(artifact.Jobs), azureTemplateDepth)
	for _, job := range artifact.Jobs {
		assert.Equal(t, "trivy fs .", job.Run)
		assert.Equal(t, "Test", job.Name)
		assert.True(t, job.ContinueOnError)
	}
	assert.Equal(t, []models.Trigger{{Event: "pull_request"}}, artifact.Triggers)
}

func TestAzureToleratesEntriesItCannotRead(t *testing.T) {
	artifact := parseAzure(t, map[string]string{"azure-pipelines.yml": `
stages:
  - [not, a, stage]
  - stage: A
    jobs:
      - [not, a, job]
      - job: B
        strategy: [odd]
        steps:
          - [not, a, step]
          - task: Foo@2
            inputs: nope
          - task: Bar
          - publish: drop
          - script: semgrep ci
`})
	require.Len(t, artifact.Jobs, 2)
	assert.Equal(t, "Bar", artifact.Jobs[0].Package)
	assert.Empty(t, artifact.Jobs[0].Version)
	assert.Nil(t, artifact.Jobs[0].With)
	assert.Equal(t, "semgrep ci", artifact.Jobs[1].Run)
}

func TestAzureExistsAndErrors(t *testing.T) {
	dir := t.TempDir()
	p := NewAzureParser()
	assert.False(t, p.Exists(dir))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "azure-pipelines.yml"), 0o755))
	assert.False(t, p.Exists(dir), "a directory with the name is not a pipeline")

	dir = testrepo.Write(t, map[string]string{"azure-pipelines.yml": "steps: [\n"})
	assert.True(t, p.Exists(dir))
	artifacts, err := p.Parse(dir)
	require.NoError(t, err)
	require.Len(t, artifacts, 1)
	assert.NotEmpty(t, artifacts[0].Skipped)
	assert.Empty(t, artifacts[0].Jobs)
}
