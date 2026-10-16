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

const bitbucketPipelines = `
image: node:20

definitions:
  steps:
    - step: &scan
        name: Scan
        image:
          name: aquasec/trivy
          username: $USER
        script:
          - trivy fs --exit-code 1 .
    - step: &secrets
        name: Secrets
        script:
          - pipe: atlassian/git-secrets-scan:0.6.1

pipelines:
  default:
    - step:
        name: Test
        script:
          - npm ci
          - npm test
        after-script:
          - npx eslint .
          - pipe: snyk/snyk-scan:1.0.1
  branches:
    main: &main
      - step: *scan
      - stage:
          name: Release
          trigger: manual
          steps:
            - step:
                name: Deploy
                script:
                  - ./deploy.sh
  bookmarks:
    stable: *main
  pull-requests:
    '**':
      - parallel:
          - step: *secrets
          - step:
              <<: *scan
              name: Scan again
      - parallel:
          fail-fast: true
          steps:
            - step:
                name: Snyk
                script:
                  - pipe: docker://snyk/snyk-scan:1.0.1
                    variables:
                      SNYK_TOKEN: $SNYK_TOKEN
                      DONT_BREAK_BUILD: "true"
                      LIST: [a, b]
  tags:
    'v*':
      - step:
          name: Sign
          script:
            - cosign sign $IMAGE
  custom:
    nightly:
      - variables:
          - name: TARGET
      - step:
          name: Nightly
          trigger: manual
          script:
            - semgrep ci
    shared:
      import: shared-pipelines:master:nightly
`

func parseBitbucket(t *testing.T, content string) []*models.Artifact {
	t.Helper()
	artifacts, err := NewBitbucketParser().Parse(testrepo.Write(t, map[string]string{bitbucketFile: content}))
	require.NoError(t, err)
	return artifacts
}

func byEvent(artifacts []*models.Artifact, event string) []*models.Artifact {
	var found []*models.Artifact
	for _, a := range artifacts {
		if a.HasTrigger(event) {
			found = append(found, a)
		}
	}
	return found
}

func TestBitbucketPipelinesGetTheirTriggers(t *testing.T) {
	artifacts := parseBitbucket(t, bitbucketPipelines)

	// The imported custom pipeline yields nothing.
	assert.Len(t, artifacts, 6)
	assert.Empty(t, artifacts[0].Triggers, "whether the default pipeline gates a merge is not in the file")
	assert.Len(t, byEvent(artifacts, "push"), 2)
	assert.Len(t, byEvent(artifacts, "pull_request"), 1)
	assert.Len(t, byEvent(artifacts, "tag"), 1)
	assert.Len(t, byEvent(artifacts, "workflow_dispatch"), 1)
	for _, a := range artifacts {
		assert.Equal(t, "Bitbucket Pipelines", a.Name)
		assert.Equal(t, bitbucketFile, filepath.Base(a.Location))
	}
	main, stable := byEvent(artifacts, "push")[0], byEvent(artifacts, "push")[1]
	assert.Equal(t, []string{"main"}, main.Triggers[0].Branches)
	assert.Equal(t, []string{"stable"}, stable.Triggers[0].Branches)
	assert.Equal(t, main.Jobs, stable.Jobs, "an aliased pipeline is read in full")
}

func TestBitbucketDefaultPipelineSplitsAfterScript(t *testing.T) {
	jobs := parseBitbucket(t, bitbucketPipelines)[0].Jobs
	require.Len(t, jobs, 3)

	assert.Equal(t, &models.Job{Name: "Test", Package: "node:20", Run: "npm ci\nnpm test"}, jobs[0])
	assert.Equal(t, "npx eslint .", jobs[1].Run)
	assert.True(t, jobs[1].ContinueOnError, "a failing after-script does not fail the step")
	assert.Equal(t, "snyk/snyk-scan", jobs[2].Package)
	assert.True(t, jobs[2].ContinueOnError)
}

func TestBitbucketAnchorsStagesAndManualTriggers(t *testing.T) {
	main := byEvent(parseBitbucket(t, bitbucketPipelines), "push")[0]
	require.Len(t, main.Jobs, 2)

	assert.Equal(t, "Scan", main.Jobs[0].Name)
	assert.Equal(t, "aquasec/trivy", main.Jobs[0].Package)
	assert.Equal(t, "trivy fs --exit-code 1 .", main.Jobs[0].Run)
	assert.False(t, main.Jobs[0].Manual)

	assert.Equal(t, "Deploy", main.Jobs[1].Name)
	assert.True(t, main.Jobs[1].Manual, "a manual stage makes its steps manual")
}

func TestBitbucketParallelBlocksAndPipes(t *testing.T) {
	jobs := byEvent(parseBitbucket(t, bitbucketPipelines), "pull_request")[0].Jobs
	require.Len(t, jobs, 5)

	assert.Equal(t, "node:20", jobs[0].Package, "a step with no image runs the default one")
	assert.Equal(t, "atlassian/git-secrets-scan", jobs[1].Package)
	assert.Equal(t, "0.6.1", jobs[1].Version)
	assert.Equal(t, "Scan again", jobs[2].Name, "a merge key keeps the anchor and applies overrides")
	assert.Equal(t, "aquasec/trivy", jobs[2].Package)

	snyk := jobs[4]
	assert.Equal(t, "snyk/snyk-scan", snyk.Package)
	assert.Equal(t, "1.0.1", snyk.Version)
	assert.Equal(t, map[string]string{"SNYK_TOKEN": "$SNYK_TOKEN", "DONT_BREAK_BUILD": "true"}, snyk.With)
}

func TestBitbucketCustomPipelinesSkipVariablesAndImports(t *testing.T) {
	custom := byEvent(parseBitbucket(t, bitbucketPipelines), "workflow_dispatch")
	require.Len(t, custom, 1)
	require.Len(t, custom[0].Jobs, 1)
	assert.Equal(t, "semgrep ci", custom[0].Jobs[0].Run)
	assert.True(t, custom[0].Jobs[0].Manual)
}

func TestBitbucketToleratesEntriesItCannotRead(t *testing.T) {
	artifacts := parseBitbucket(t, `
pipelines:
  default:
    - step: [not, a, step]
    - parallel: {steps: oops}
    - step:
        script:
          - 42
          - {not: a pipe}
          - ""
          - gitleaks detect
`)
	require.Len(t, artifacts, 1)
	require.Len(t, artifacts[0].Jobs, 1)
	assert.Equal(t, "42\ngitleaks detect", artifacts[0].Jobs[0].Run)
}

func TestBitbucketPipeWithoutVersion(t *testing.T) {
	job := bitbucketPipeJob("s", bitbucketPipe{Pipe: "docker://registry:5000/acme/scan"}, false, false)
	assert.Equal(t, "registry:5000/acme/scan", job.Package)
	assert.Empty(t, job.Version)
}

func TestBitbucketEmptyFileHasNoArtifacts(t *testing.T) {
	assert.Empty(t, parseBitbucket(t, "image: node:20\n"))
}

func TestBitbucketExistsAndErrors(t *testing.T) {
	dir := t.TempDir()
	p := NewBitbucketParser()
	assert.False(t, p.Exists(dir))

	_, err := p.Parse(dir)
	assert.Error(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(dir, bitbucketFile), []byte("pipelines: [\n"), 0o600))
	assert.True(t, p.Exists(dir))
	_, err = p.Parse(dir)
	assert.Error(t, err)
}

func TestBitbucketBranchTriggers(t *testing.T) {
	assert.Equal(t, []models.Trigger{{Event: "push", Branches: []string{"main"}}}, branchTriggers("main"))
	for _, pattern := range []string{"**", "feature/*", "release-?", "{main,develop}", "[a-z]*"} {
		assert.Nil(t, branchTriggers(pattern), pattern)
	}
}
