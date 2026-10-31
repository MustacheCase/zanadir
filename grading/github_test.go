package grading

import (
	"testing"

	"github.com/MustacheCase/zanadir/parser"
	"github.com/stretchr/testify/assert"
)

// A continue-on-error expression may or may not let the step fail, so the
// control is unknown - neither enforcing nor advisory.
func TestContinueOnErrorExpressionGradesUnknown(t *testing.T) {
	verdicts := gradeFile(t, parser.NewGithubParser(), ".github/workflows/ci.yml", `
on: pull_request
jobs:
  scan:
    steps:
      - run: gitleaks detect
        continue-on-error: ${{ matrix.experimental }}
      - run: semgrep ci
        continue-on-error: true
`)
	assert.Equal(t, Unknown, verdicts["Secrets Detection"])
	assert.Equal(t, Advisory, verdicts["SAST"])
}

// revive exits 0 on findings unless -set_exit_status is given, and a -config
// file can set the exit code some other way.
func TestReviveExitsZeroUnlessSetExitStatus(t *testing.T) {
	for run, expected := range map[string]Verdict{
		"revive -set_exit_status ./...":                    Enforcing,
		"revive ./...":                                     Advisory,
		"revive -config revive.toml -formatter json ./...": Unknown,
	} {
		verdicts := gradeFile(t, parser.NewGithubParser(), ".github/workflows/ci.yml", `
on: pull_request
jobs:
  lint:
    steps:
      - run: `+run+`
`)
		assert.Equal(t, expected, verdicts["Linter"], run)
	}
}

// Retire.js fails on findings unless --exitwith 0 tells it not to.
func TestRetireJSExitWithZeroGradesAdvisory(t *testing.T) {
	for run, expected := range map[string]Verdict{
		"npx retire --path dist/":              Enforcing,
		"npx retire --exitwith 1":              Enforcing,
		"npx retire --path dist/ --exitwith 0": Advisory,
	} {
		verdicts := gradeFile(t, parser.NewGithubParser(), ".github/workflows/ci.yml", `
on: pull_request
jobs:
  scan:
    steps:
      - run: `+run+`
`)
		assert.Equal(t, expected, verdicts["SCA"], run)
	}
}

func TestFlawfinderExitsZeroUnlessErrorLevel(t *testing.T) {
	for run, expected := range map[string]Verdict{
		"flawfinder --error-level=4 src/":                      Enforcing,
		"pipx run flawfinder --error-level 1 lib/":             Enforcing,
		"flawfinder --minlevel 3 --sarif . > flawfinder.sarif": Advisory,
	} {
		verdicts := gradeFile(t, parser.NewGithubParser(), ".github/workflows/ci.yml", `
on: pull_request
jobs:
  scan:
    steps:
      - run: `+run+`
`)
		assert.Equal(t, expected, verdicts["SAST"], run)
	}
}

// The flawfinder action takes its flags through arguments, which
// exitsZeroUnless can't read, so it must not claim a gate: neither on its own
// nor in GitHub's starter workflow, which is also named flawfinder.
func TestFlawfinderActionGradesNoSAST(t *testing.T) {
	for path, workflow := range map[string]string{
		".github/workflows/ci.yml": `
name: ci
on: pull_request
jobs:
  scan:
    steps:
      - uses: david-a-wheeler/flawfinder@8e4a779ad59dbfaee5da586aa9210853b701959c
        with:
          arguments: '--sarif ./'
          output: 'flawfinder_results.sarif'
`,
		".github/workflows/flawfinder.yml": `
name: flawfinder
on:
  push:
    branches: [main]
  pull_request:
    branches: [main]
jobs:
  flawfinder:
    name: Flawfinder
    runs-on: ubuntu-latest
    steps:
      - name: Checkout code
        uses: actions/checkout@v4
      - name: flawfinder_scan
        uses: david-a-wheeler/flawfinder@8e4a779ad59dbfaee5da586aa9210853b701959c
        with:
          arguments: '--sarif ./'
          output: 'flawfinder_results.sarif'
      - name: Upload analysis results to GitHub Security tab
        uses: github/codeql-action/upload-sarif@v3
        with:
          sarif_file: flawfinder_results.sarif
`,
	} {
		verdicts := gradeFile(t, parser.NewGithubParser(), path, workflow)
		assert.NotContains(t, verdicts, "SAST", path)
	}
}
