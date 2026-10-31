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
