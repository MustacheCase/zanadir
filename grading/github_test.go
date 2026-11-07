package grading

import (
	"strings"
	"testing"

	"github.com/MustacheCase/zanadir/parser"
	"github.com/stretchr/testify/assert"
)

// Executable quotes and directory prefixes must not change either recognition
// or the command-specific evidence used to grade a control.
func TestQuotedToolPathsGradeLikeBareCommands(t *testing.T) {
	for _, tt := range []struct {
		name, run, category string
		want                Verdict
	}{
		{"bare grant", "grant check dir:.", "License Compliance", Enforcing},
		{"relative grant", "./bin/grant check dir:.", "License Compliance", Enforcing},
		{"absolute grant", "/opt/bin/grant check dir:.", "License Compliance", Enforcing},
		{"double quoted grant", `"$RUNNER_TEMP/grant" check dir:.`, "License Compliance", Enforcing},
		{"single quoted grant", `'./bin/grant' check dir:.`, "License Compliance", Enforcing},
		{"grant path with spaces", `"/opt/tool cache/grant" check dir:.`, "License Compliance", Enforcing},
		{"grant list", `"./bin/grant" list dir:.`, "License Compliance", Enforcing},
		{"swallowed grant", `"$RUNNER_TEMP/grant" check dir:. || true`, "License Compliance", Advisory},
		{"reraised grant", `"$RUNNER_TEMP/grant" check dir:. || exit 1`, "License Compliance", Enforcing},
		{"conditional grant", `if "./bin/grant" check dir:.; then echo done; fi`, "License Compliance", Advisory},
		{"bare semgrep ci", "semgrep ci", "SAST", Enforcing},
		{"quoted semgrep ci", `"./bin/semgrep" ci`, "SAST", Enforcing},
		{"quoted semgrep scan", `"./bin/semgrep" scan`, "SAST", Advisory},
		{"quoted semgrep error", `"./bin/semgrep" scan --error`, "SAST", Enforcing},
		{"quoted trivy fails", `"./bin/trivy" fs --exit-code 1 .`, "SCA", Enforcing},
		{"quoted trivy reports", `"./bin/trivy" fs .`, "SCA", Advisory},
		{"quoted trivy zero", `"./bin/trivy" fs --exit-code 0 .`, "SCA", Advisory},
		{"trivy environment retained", `TRIVY_EXIT_CODE=1 "./bin/trivy" fs .`, "SCA", Enforcing},
		{"neighbor flag cannot enforce", "gitleaks detect --exit-code 1\n\"./bin/trivy\" fs .", "SCA", Advisory},
		{"neighbor flag cannot weaken", "gitleaks detect --exit-code 0\n\"./bin/trivy\" fs --exit-code 1 .", "SCA", Enforcing},
		{"neighbor failure is unrelated", "gitleaks detect || true\n\"./bin/grant\" check dir:.", "License Compliance", Enforcing},
	} {
		t.Run(tt.name, func(t *testing.T) {
			verdicts := gradeFile(t, parser.NewGithubParser(), ".github/workflows/ci.yml", `
on: pull_request
jobs:
  check:
    steps:
      - run: |
          `+strings.ReplaceAll(tt.run, "\n", "\n          ")+`
`)
			assert.Equal(t, tt.want, verdicts[tt.category])
		})
	}
}

func TestGrantQuotedPathInYAMLScalar(t *testing.T) {
	verdicts := gradeFile(t, parser.NewGithubParser(), ".github/workflows/ci.yml", `
on: pull_request
jobs:
  check:
    steps:
      - run: '"$RUNNER_TEMP/grant" check dir:.'
`)
	assert.Equal(t, Enforcing, verdicts["License Compliance"])
}

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

// license-checker only lists licences unless --failOn or --onlyAllow makes it
// fail; a --config file can set either, so that run is unknown.
func TestLicenseCheckerExitsZeroUnlessFailFlag(t *testing.T) {
	for run, expected := range map[string]Verdict{
		"npx license-checker --json --out licenses.json":                      Advisory,
		"npx license-checker --production --onlyAllow \"MIT;ISC;Apache-2.0\"": Enforcing,
		"license-checker --failOn \"GPL-3.0;AGPL-3.0\"":                       Enforcing,
		"npx license-checker --config .licenses.json":                         Unknown,
		"npx license-checker --config=.licenses.json --json":                  Unknown,
	} {
		verdicts := gradeFile(t, parser.NewGithubParser(), ".github/workflows/ci.yml", `
on: pull_request
jobs:
  licenses:
    steps:
      - run: `+run+`
`)
		assert.Equal(t, expected, verdicts["License Compliance"], run)
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

// nancy fails on findings unless --no-fail tells it not to.
func TestNancyNoFailGradesAdvisory(t *testing.T) {
	for run, expected := range map[string]Verdict{
		"go list -json -deps ./... | nancy sleuth":           Enforcing,
		"go list -json -deps ./... | nancy sleuth --no-fail": Advisory,
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

// CIFuzz fails on a crash unless dry-run tells it only to report.
func TestCIFuzzDryRunGradesAdvisory(t *testing.T) {
	for with, expected := range map[string]Verdict{
		"": Enforcing,
		"\n        with:\n          dry-run: true": Advisory,
	} {
		verdicts := gradeFile(t, parser.NewGithubParser(), ".github/workflows/cifuzz.yml", `
on: pull_request
jobs:
  fuzz:
    steps:
      - uses: google/oss-fuzz/infra/cifuzz/actions/run_fuzzers@master`+with+`
`)
		assert.Equal(t, expected, verdicts["SAST"], with)
	}
}

// GuardDog reports findings without failing unless explicitly asked to fail.
func TestGuardDogExitsZeroUnlessFindingFlag(t *testing.T) {
	for run, expected := range map[string]Verdict{
		"uvx guarddog pypi verify requirements.txt --exit-non-zero-on-finding":         Enforcing,
		"uvx guarddog@2.8.0 pypi verify requirements.txt --exit-non-zero-on-finding":   Enforcing,
		"guarddog npm scan express --exit-non-zero-on-finding":                         Enforcing,
		"guarddog npm scan express":                                                    Advisory,
		"guarddog github_action verify .github/workflows/ci.yml --output-format sarif": Advisory,
		"docker run --rm ghcr.io/datadog/guarddog:latest pypi scan requests":           Advisory,
		"guarddog npm scan express --exit-non-zero-on-finding-extra":                   Advisory,
		"guarddog npm scan express ${{ inputs.args }}":                                 Unknown,
		"guarddog npm scan express --exit-non-zero-on-finding || true":                 Advisory,
	} {
		t.Run(run, func(t *testing.T) {
			verdicts := gradeFile(t, parser.NewGithubParser(), ".github/workflows/ci.yml", `
on: pull_request
jobs:
  scan:
    steps:
      - run: `+run+`
`)
			assert.Equal(t, expected, verdicts["SCA"])
		})
	}
}

// A dependency scan on push runs after merge, so it cannot fully gate changes.
func TestGuardDogPushOnlyGradesPartial(t *testing.T) {
	verdicts := gradeFile(t, parser.NewGithubParser(), ".github/workflows/guarddog.yml", `
name: GuardDog
on: push
jobs:
  scan:
    steps:
      - run: guarddog npm scan express --exit-non-zero-on-finding
`)
	assert.Equal(t, Partial, verdicts["SCA"])
	assert.NotContains(t, verdicts, "Supply Chain")
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

// The action only reports without --error-level, even in the starter workflow
// named flawfinder: neither the workflow name nor the SARIF upload is a gate.
func TestFlawfinderActionWithoutErrorLevelGradesAdvisory(t *testing.T) {
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
		assert.Equal(t, Advisory, verdicts["SAST"], path)
	}
}

func TestFlawfinderActionArguments(t *testing.T) {
	tests := []struct {
		name     string
		event    string
		options  string
		expected Verdict
	}{
		{"error level with equals", "pull_request", "        with:\n          arguments: '--error-level=4 --sarif ./'\n", Enforcing},
		{"error level with space", "pull_request", "        with:\n          arguments: '--error-level 4 --sarif ./'\n", Enforcing},
		{"no arguments", "pull_request", "", Advisory},
		{"empty arguments", "pull_request", "        with:\n          arguments: ''\n", Advisory},
		{"another input", "pull_request", "        with:\n          output: '--error-level=4.sarif'\n", Advisory},
		{"expression", "pull_request", "        with:\n          arguments: ${{ inputs.args }}\n", Unknown},
		{"conditional flag", "pull_request", "        with:\n          arguments: ${{ inputs.fail && '--error-level=4' || '--sarif ./' }}\n", Unknown},
		{"push only", "push", "        with:\n          arguments: '--error-level=4'\n", Partial},
		{"continue on error", "pull_request", "        continue-on-error: true\n        with:\n          arguments: '--error-level=4'\n", Advisory},
		{"condition", "pull_request", "        if: github.event_name == 'pull_request'\n        with:\n          arguments: '--error-level=4'\n", Unknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			verdicts := gradeFile(t, parser.NewGithubParser(), ".github/workflows/flawfinder.yml", `
name: flawfinder
on: `+tt.event+`
jobs:
  scan:
    steps:
      - uses: david-a-wheeler/flawfinder@8e4a779ad59dbfaee5da586aa9210853b701959c
`+tt.options)
			assert.Equal(t, tt.expected, verdicts["SAST"])
		})
	}
}

func TestBufActionLintInputGradesCorrectly(t *testing.T) {
	for _, tc := range []struct {
		name     string
		input    string
		expected Verdict
	}{
		{name: "default lint is enforcing", expected: Enforcing},
		{name: "lint disabled is advisory", input: "lint: false", expected: Advisory},
		{name: "expression is unknown", input: "lint: ${{ inputs.lint }}", expected: Unknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			workflow := `on: pull_request
jobs:
  lint:
    steps:
      - uses: bufbuild/buf-action@v1
`
			if tc.input != "" {
				workflow += "        with:\n          " + tc.input + "\n"
			}
			verdicts := gradeFile(t, parser.NewGithubParser(), ".github/workflows/ci.yml", workflow)
			assert.Equal(t, tc.expected, verdicts["Linter"])
		})
	}
}

func TestCodespellActionOnlyWarnInputGradesCorrectly(t *testing.T) {
	for _, tc := range []struct {
		name     string
		input    string
		expected Verdict
	}{
		{name: "default is enforcing", expected: Enforcing},
		{name: "only_warn 1 is advisory", input: "only_warn: 1", expected: Advisory},
		{name: "only_warn true is advisory", input: "only_warn: true", expected: Advisory},
		{name: "expression is unknown", input: "only_warn: ${{ inputs.only_warn }}", expected: Unknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			workflow := `on: pull_request
jobs:
  spelling:
    steps:
      - uses: codespell-project/actions-codespell@v2
`
			if tc.input != "" {
				workflow += "        with:\n          " + tc.input + "\n"
			}
			verdicts := gradeFile(t, parser.NewGithubParser(), ".github/workflows/ci.yml", workflow)
			assert.Equal(t, tc.expected, verdicts["Linter"])
		})
	}
}
