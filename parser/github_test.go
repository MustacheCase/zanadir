package parser_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/MustacheCase/zanadir/internal/testrepo"
	"github.com/MustacheCase/zanadir/models"
	"github.com/MustacheCase/zanadir/parser"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testWorkflow = `
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

// workflows creates a repository with these files in .github/workflows.
func workflows(t *testing.T, files map[string]string) string {
	t.Helper()
	inDir := make(map[string]string, len(files))
	for name, content := range files {
		inDir[filepath.Join(".github", "workflows", name)] = content
	}
	return testrepo.Write(t, inDir)
}

func TestGithubExists(t *testing.T) {
	gp := parser.NewGithubParser()
	assert.True(t, gp.Exists(workflows(t, map[string]string{"test-workflow.yml": testWorkflow})))
	assert.False(t, gp.Exists("nonexistent-dir"))
}

func TestGithubParse(t *testing.T) {
	artifacts, err := parser.NewGithubParser().Parse(workflows(t, map[string]string{"test-workflow.yml": testWorkflow}))
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
	dir := workflows(t, map[string]string{"ci.yml": `
name: ci
jobs:
  security:
    steps:
      - uses: actions/checkout@v4
      - name: Scan
        run: trivy fs --exit-code 1 .
`})

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
	artifacts, err := parser.NewGithubParser().Parse(workflows(t, map[string]string{"wf.yml": workflow}))
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

// A matrix build lets some cells fail with an expression. Decoding the field
// as a bool aborted the whole scan; 3 of the 54 most-starred repositories
// tried did this. The expression is kept as a condition grading cannot read.
func TestParseContinueOnErrorExpressions(t *testing.T) {
	artifact := parseOne(t, `
name: w
on: pull_request
jobs:
  a:
    continue-on-error: ${{ matrix.experimental }}
    if: github.event_name == 'pull_request'
    steps:
      - run: gitleaks detect
        continue-on-error: ${{ github.ref != 'refs/heads/main' }}
      - run: trivy fs .
        continue-on-error: "true"
      - run: semgrep ci
        continue-on-error: "False"
`)
	gitleaks, trivy, semgrep := artifact.Jobs[0], artifact.Jobs[1], artifact.Jobs[2]

	assert.False(t, gitleaks.ContinueOnError)
	assert.Equal(t, "continue-on-error: ${{ github.ref != 'refs/heads/main' }}", gitleaks.If)
	assert.Equal(t, "github.event_name == 'pull_request' && continue-on-error: ${{ matrix.experimental }}", gitleaks.JobIf)

	assert.True(t, trivy.ContinueOnError, "a quoted true is still true")
	assert.Empty(t, trivy.If)
	assert.False(t, semgrep.ContinueOnError)
	assert.Empty(t, semgrep.If)
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

// These workflows come from repositories zanadir did not write. A field it
// cannot read must degrade to empty, never abort the scan: an earlier version
// of this parser returned an error for "needs: [build, {b: c}]", which took
// the whole repository's scan down with one odd workflow.
func TestParseToleratesMalformedFields(t *testing.T) {
	tests := []struct {
		name             string
		workflow         string
		expectedTriggers []models.Trigger
		expectedNeeds    []string
	}{
		{
			name:             "a needs sequence with a non-scalar entry keeps the rest",
			workflow:         "name: w\non: push\njobs:\n  a:\n    needs: [build, {b: c}]\n    steps:\n      - run: z\n",
			expectedTriggers: []models.Trigger{{Event: "push"}},
			expectedNeeds:    []string{"build"},
		},
		{
			name:             "a needs mapping is ignored",
			workflow:         "name: w\non: push\njobs:\n  a:\n    needs:\n      x: y\n    steps:\n      - run: z\n",
			expectedTriggers: []models.Trigger{{Event: "push"}},
			expectedNeeds:    nil,
		},
		{
			name:             "a trigger list with a non-scalar entry keeps the rest",
			workflow:         "name: w\non: [push, {a: b}]\njobs:\n  a:\n    steps:\n      - run: z\n",
			expectedTriggers: []models.Trigger{{Event: "push"}},
			expectedNeeds:    nil,
		},
		{
			name:             "an empty trigger scalar yields none",
			workflow:         "name: w\non: \"\"\njobs:\n  a:\n    steps:\n      - run: z\n",
			expectedTriggers: nil,
			expectedNeeds:    nil,
		},
		{
			name:             "an empty event name is skipped",
			workflow:         "name: w\non:\n  \"\": {}\n  push:\njobs:\n  a:\n    steps:\n      - run: z\n",
			expectedTriggers: []models.Trigger{{Event: "push"}},
			expectedNeeds:    nil,
		},
		{
			name:             "a branch filter that is a mapping is ignored",
			workflow:         "name: w\non:\n  push:\n    branches:\n      x: y\njobs:\n  a:\n    steps:\n      - run: z\n",
			expectedTriggers: []models.Trigger{{Event: "push"}},
			expectedNeeds:    nil,
		},
		{
			name:             "a needs sequence with no readable entry yields none",
			workflow:         "name: w\non: push\njobs:\n  a:\n    needs: [{a: b}]\n    steps:\n      - run: z\n",
			expectedTriggers: []models.Trigger{{Event: "push"}},
			expectedNeeds:    nil,
		},
		{
			name:             "a mapping of only empty event names yields none",
			workflow:         "name: w\non:\n  \"\": {}\njobs:\n  a:\n    steps:\n      - run: z\n",
			expectedTriggers: nil,
			expectedNeeds:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			artifact := parseOne(t, tt.workflow)
			assert.Equal(t, tt.expectedTriggers, artifact.Triggers)
			assert.Len(t, artifact.Jobs, 1)
			assert.Equal(t, tt.expectedNeeds, artifact.Jobs[0].Needs)
		})
	}
}

// A step with neither run nor uses - a bare name, or a step commented down to
// nothing - carries no tool and must not become an empty job record.
func TestParseSkipsStepsWithNoTool(t *testing.T) {
	artifact := parseOne(t, `
name: w
on: push
jobs:
  a:
    steps:
      - name: just a label
      - run: gitleaks detect
`)
	assert.Len(t, artifact.Jobs, 1)
	assert.Equal(t, "gitleaks detect", artifact.Jobs[0].Run)
}

func TestGithubExistsRejectsADirectoryWithNoWorkflow(t *testing.T) {
	assert.False(t, parser.NewGithubParser().Exists(t.TempDir()))
	assert.False(t, parser.NewGithubParser().Exists(workflows(t, map[string]string{"README.md": "x"})))
}

func TestParseSkipsSubdirectories(t *testing.T) {
	dir := workflows(t, map[string]string{
		"wf.yml":            "name: w\non: push\njobs:\n  a:\n    steps:\n      - run: z\n",
		"nested/inside.yml": "name: n\non: push\njobs:\n  a:\n    steps:\n      - run: z\n",
	})

	artifacts, err := parser.NewGithubParser().Parse(dir)
	assert.NoError(t, err)
	assert.Len(t, artifacts, 1)
}

// GitHub refuses to run a workflow that is not valid YAML, so it is skipped
// with the reason and the other workflows are still read.
func TestParseSkipsUnparseableYAML(t *testing.T) {
	artifacts, err := parser.NewGithubParser().Parse(workflows(t, map[string]string{
		"broken.yml":    "name: w\n  jobs: [oops\n",
		"duplicate.yml": "jobs:\n  build:\n    if: false\n    if: true\n",
		"empty.yml":     "",
		"lint.yml":      "jobs:\n  lint:\n    steps:\n      - uses: golangci/golangci-lint-action@v6\n",
	}))
	require.NoError(t, err)
	skipped := map[string]string{}
	jobs := 0
	for _, a := range artifacts {
		skipped[filepath.Base(a.Location)] = a.Skipped
		jobs += len(a.Jobs)
	}
	assert.Contains(t, skipped["broken.yml"], "line 2")
	assert.Equal(t, `it is not valid YAML (yaml: unmarshal errors: line 4: mapping key "if" already defined at line 3), so the CI platform cannot run it either`, skipped["duplicate.yml"])
	assert.Empty(t, skipped["empty.yml"], "an empty file is valid, if pointless")
	assert.Empty(t, skipped["lint.yml"])
	assert.Equal(t, 1, jobs)
}

// Grading against required checks needs the name each job reports under.
func TestGithubParserRecordsCheckNames(t *testing.T) {
	artifact := parseOne(t, `on: pull_request
jobs:
  lint:
    name: Static analysis
    steps:
      - run: golangci-lint run
  unit:
    steps:
      - run: go test ./...
  codeql:
    uses: ./.github/workflows/codeql.yml
    if: github.event_name == 'pull_request'
`)
	checks := map[string]string{}
	for _, job := range artifact.Jobs {
		checks[job.Name] = job.Check
	}
	assert.Equal(t, map[string]string{"lint": "Static analysis", "unit": "unit", "codeql": "codeql"}, checks)
}

func byFile(t *testing.T, files map[string]string) map[string]*models.Artifact {
	t.Helper()
	artifacts, err := parser.NewGithubParser().Parse(workflows(t, files))
	require.NoError(t, err)
	out := map[string]*models.Artifact{}
	for _, a := range artifacts {
		out[filepath.Base(a.Location)] = a
	}
	return out
}

func events(a *models.Artifact) []string {
	var names []string
	for _, trigger := range a.Triggers {
		names = append(names, trigger.Event)
	}
	return names
}

// on: workflow_call says nothing about when a workflow runs; its callers do.
func TestReusableWorkflowsInheritTheirCallers(t *testing.T) {
	got := byFile(t, map[string]string{
		"ci.yml":      "on: pull_request\njobs:\n  security:\n    uses: ./.github/workflows/scan.yml\n  remote:\n    uses: org/sec/.github/workflows/gitleaks.yml@v1\n",
		"scan.yml":    "on:\n  workflow_call:\n  schedule: [{cron: '0 3 * * *'}]\njobs:\n  leaks:\n    name: Leaks\n    runs-on: x\n    steps:\n      - run: gitleaks detect\n  deeper:\n    uses: ./.github/workflows/deep.yml\n",
		"deep.yml":    "on: workflow_call\njobs:\n  sast:\n    runs-on: x\n    steps:\n      - run: semgrep scan\n",
		"release.yml": "on: {push: {tags: ['v*']}}\njobs:\n  a:\n    uses: ./.github/workflows/sign.yml\n  b:\n    if: github.repository == 'o/r'\n    continue-on-error: true\n    uses: ./.github/workflows/sign.yml\n",
		"sign.yml":    "on: workflow_call\njobs:\n  cosign:\n    runs-on: x\n    steps:\n      - run: cosign sign\n",
		"soft.yml":    "on: pull_request\njobs:\n  a:\n    continue-on-error: true\n    uses: ./.github/workflows/lint.yml\n",
		"lint.yml":    "on: workflow_call\njobs:\n  lint:\n    runs-on: x\n    steps:\n      - run: golangci-lint run\n",
		"self.yml":    "on: workflow_call\njobs:\n  again:\n    uses: ./.github/workflows/self.yml\n",
	})

	assert.ElementsMatch(t, []string{"workflow_call", "schedule", "pull_request"}, events(got["scan.yml"]))
	assert.ElementsMatch(t, []string{"workflow_call", "schedule", "pull_request"}, events(got["deep.yml"]), "through a chain of calls")
	assert.Equal(t, []string{"workflow_call"}, events(got["self.yml"]))

	job := func(file, name string) *models.Job {
		for _, j := range got[file].Jobs {
			if j.Name == name {
				return j
			}
		}
		t.Fatalf("%s has no job %s", file, name)
		return nil
	}
	assert.Equal(t, "security / Leaks", job("scan.yml", "leaks").Check, "one caller: checks report as caller / job")
	assert.Equal(t, "security / deeper", job("scan.yml", "deeper").Check)
	assert.Equal(t, "cosign", job("sign.yml", "cosign").Check, "two callers: no single prefix")
	assert.Equal(t, "github.repository == 'o/r'", job("sign.yml", "cosign").JobIf, "a caller's condition carries over")
	assert.False(t, job("sign.yml", "cosign").ContinueOnError, "one caller can still fail the build")
	assert.True(t, job("lint.yml", "lint").ContinueOnError, "every caller may fail")

	remote := job("ci.yml", "remote")
	assert.Equal(t, "org/sec/.github/workflows/gitleaks.yml", remote.Package, "a remote reusable workflow can be matched by its path")
	assert.Equal(t, "v1", remote.Version)
}

// django runs flake8 through liskin/gh-problem-matcher-wrap's run: input.
func TestGithubParserReadsACommandPassedToAnAction(t *testing.T) {
	artifact := parseOne(t, `on: pull_request
jobs:
  lint:
    runs-on: ubuntu-latest
    steps:
      - uses: liskin/gh-problem-matcher-wrap@v3
        with:
          linters: flake8
          run: flake8
      - uses: nick-fields/retry@v3
        with:
          command: go test ./...
      - uses: actions/github-script@v7
        with:
          script: console.log("not a shell command")
`)
	runs := map[string]string{}
	for _, job := range artifact.Jobs {
		runs[job.Package] = job.Run
	}
	assert.Equal(t, "flake8", runs["liskin/gh-problem-matcher-wrap"])
	assert.Equal(t, "go test ./...", runs["nick-fields/retry"])
	assert.Empty(t, runs["actions/github-script"])
}

func TestGithubParserReadsGoreleaserActionArgs(t *testing.T) {
	artifact := parseOne(t, "on: {push: {tags: ['v*']}}\njobs:\n  release:\n    runs-on: x\n    steps:\n      - uses: goreleaser/goreleaser-action@v6\n        with:\n          args: release --clean\n      - uses: goreleaser/goreleaser-action@v6\n")
	var runs []string
	for _, job := range artifact.Jobs {
		runs = append(runs, job.Run)
	}
	assert.ElementsMatch(t, []string{"goreleaser release --clean", "goreleaser"}, runs)
}

func TestGithubParserReadsTrivyActionAsItsCommand(t *testing.T) {
	artifact := parseOne(t, "on: pull_request\njobs:\n  scan:\n    runs-on: x\n    steps:\n      - uses: aquasecurity/trivy-action@v0.36.0\n      - uses: aquasecurity/trivy-action@v0.36.0\n        with:\n          scan-type: config\n          exit-code: '1'\n      - uses: aquasecurity/trivy-action@v0.36.0\n        with:\n          scan-type: fs\n          scanners: secret\n          format: sarif\n      - uses: aquasecurity/trivy-action@v0.36.0\n        with:\n          scan-type: ${{ matrix.scan-type }}\n")
	var runs []string
	for _, job := range artifact.Jobs {
		runs = append(runs, job.Run)
	}
	assert.Equal(t, []string{"trivy image", "trivy config --exit-code 1", "trivy fs --scanners secret --format sarif", "trivy image"}, runs, "an expression is taken as the default")
}

func TestGithubParserReadsPullRequestFilters(t *testing.T) {
	artifact := parseOne(t, "on:\n  pull_request:\n    types: [opened, closed]\n    branches-ignore: [docs/**]\n  push:\n    branches: [main]\njobs:\n  a:\n    runs-on: x\n    steps:\n      - run: echo\n")
	assert.Equal(t, []models.Trigger{
		{Event: "pull_request", IgnoredBranches: []string{"docs/**"}, Types: []string{"opened", "closed"}},
		{Event: "push", Branches: []string{"main"}},
	}, artifact.Triggers)
}

// One entry the scan cannot read used to abort it; like invalid YAML, it is
// skipped with the reason, and the other workflows are still read.
func TestParseSkipsWorkflowsItCannotRead(t *testing.T) {
	root := workflows(t, map[string]string{"lint.yml": "on: pull_request\njobs:\n  lint:\n    steps:\n      - run: golangci-lint run\n"})
	dir := filepath.Join(root, ".github", "workflows")
	require.NoError(t, os.Symlink("/nonexistent/x.yml", filepath.Join(dir, "dangling.yml")))
	require.NoError(t, os.Symlink("loop.yml", filepath.Join(dir, "loop.yml")))
	require.NoError(t, os.Mkdir(filepath.Join(root, "adir"), 0o755))
	require.NoError(t, os.Symlink(filepath.Join(root, "adir"), filepath.Join(dir, "dir.yml")))
	locked := filepath.Join(dir, "locked.yml")
	require.NoError(t, os.WriteFile(locked, []byte("on: push\n"), 0o000))

	artifacts, err := parser.NewGithubParser().Parse(root)
	require.NoError(t, err)
	reasons := map[string]string{}
	for _, a := range artifacts {
		reasons[filepath.Base(a.Location)] = a.Skipped
	}
	assert.Empty(t, reasons["lint.yml"])
	for _, name := range []string{"dangling.yml", "loop.yml", "dir.yml"} {
		assert.Contains(t, reasons[name], "it cannot be read", name)
	}
	if os.Geteuid() != 0 {
		assert.Contains(t, reasons["locked.yml"], "permission denied")
	}
}
