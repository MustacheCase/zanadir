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

// pinact run changes workflow files by default, but --check, --diff and
// --fix=false turn unpinned actions into a failing CI check.
func TestPinactRunGradesOnlyCheckingModesAsEnforcing(t *testing.T) {
	for run, expected := range map[string]Verdict{
		"pinact run":                      Advisory,
		"pinact run --check":              Enforcing,
		"pinact run --diff":               Enforcing,
		"pinact run --fix=false --no-api": Enforcing,
		"aqua exec -- pinact run --check": Enforcing,
		"pinact run --fix false":          Advisory,
		"pinact run --checkout-dir x":     Advisory,
		"pinact run --verify-comment":     Advisory,
		"pinact run --checkmate":          Advisory,
		"pinact run --difference":         Advisory,
		"pinact run --fix=falsehood":      Advisory,
	} {
		verdicts := gradeFile(t, parser.NewGithubParser(), ".github/workflows/pinact.yml", `
on: pull_request
jobs:
  pin:
    steps:
      - run: `+run+`
`)
		assert.Equal(t, expected, verdicts["Workflow Security"], run)
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

// Vale exits 1 on an error-level alert, so the command gates unless --no-exit
// turns that off. A vale sync step only downloads styles and must not make a
// job that also runs vale --no-exit look enforcing.
func TestValeCommandGradesByExitCode(t *testing.T) {
	for run, expected := range map[string]Verdict{
		"vale docs/":                       Enforcing,
		"vale --minAlertLevel=error docs/": Enforcing,
		"vale --no-exit docs/":             Advisory,
	} {
		t.Run(run, func(t *testing.T) {
			verdicts := gradeFile(t, parser.NewGithubParser(), ".github/workflows/ci.yml", `
on: pull_request
jobs:
  docs:
    steps:
      - run: `+run+`
`)
			assert.Equal(t, expected, verdicts["Linter"])
		})
	}

	t.Run("sync then no-exit", func(t *testing.T) {
		verdicts := gradeFile(t, parser.NewGithubParser(), ".github/workflows/ci.yml", `
on: pull_request
jobs:
  docs:
    steps:
      - run: vale sync
      - run: vale --no-exit docs/
`)
		assert.Equal(t, Advisory, verdicts["Linter"])
	})
}

// vale-action ignores Vale's exit code and fails the step only with
// fail_on_error: true; fail_level: none never fails even then.
func TestValeActionFailOnErrorGradesCorrectly(t *testing.T) {
	for _, tc := range []struct {
		name, uses, with string
		want             Verdict
	}{
		{"default advisory", "vale-cli/vale-action@v3", "", Advisory},
		{"fail on error true", "vale-cli/vale-action@v3", "fail_on_error: true", Enforcing},
		{"fail on error false", "vale-cli/vale-action@v3", "fail_on_error: false", Advisory},
		{"fail level none", "vale-cli/vale-action@v3", "fail_on_error: true\n          fail_level: none", Advisory},
		{"fail on error expression", "vale-cli/vale-action@v3", "fail_on_error: ${{ inputs.strict }}", Unknown},
		{"former name", "errata-ai/vale-action@reviewdog", "fail_on_error: true", Enforcing},
		{"grafana wrapper", "grafana/writers-toolkit/vale-action@vale-action/v2", "fail_on_error: true", Enforcing},
	} {
		t.Run(tc.name, func(t *testing.T) {
			workflow := `on: pull_request
jobs:
  docs:
    steps:
      - uses: ` + tc.uses + "\n"
			if tc.with != "" {
				workflow += "        with:\n          " + tc.with + "\n"
			}
			verdicts := gradeFile(t, parser.NewGithubParser(), ".github/workflows/ci.yml", workflow)
			assert.Equal(t, tc.want, verdicts["Linter"])
		})
	}
}

// Secretlint does not fail on findings when writing its report with --output.
func TestSecretlintOutputGradesAdvisory(t *testing.T) {
	for run, expected := range map[string]Verdict{
		`npx secretlint "**/*"`:                                    Enforcing,
		"secretlint --format sarif .":                                Enforcing,
		`npx secretlint "**/*" --format sarif --output secretlint.sarif`: Advisory,
		"secretlint --output=report.json .":                          Advisory,
		"secretlint --output-format sarif .":                          Enforcing,
	} {
		t.Run(run, func(t *testing.T) {
			verdicts := gradeFile(t, parser.NewGithubParser(), ".github/workflows/ci.yml", `
on: pull_request
jobs:
  scan:
    steps:
      - run: `+run+`
`)
			assert.Equal(t, expected, verdicts["Secrets Detection"])
		})
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

// lychee-action fails the build on a broken link unless fail is false.
func TestLycheeActionFailInputGradesCorrectly(t *testing.T) {
	for _, tc := range []struct {
		name     string
		input    string
		expected Verdict
	}{
		{name: "default fail is enforcing", expected: Enforcing},
		{name: "fail disabled is advisory", input: "fail: false", expected: Advisory},
		{name: "expression is unknown", input: "fail: ${{ inputs.fail }}", expected: Unknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			workflow := `on: pull_request
jobs:
  links:
    steps:
      - uses: lycheeverse/lychee-action@v2
`
			if tc.input != "" {
				workflow += "        with:\n          " + tc.input + "\n"
			}
			verdicts := gradeFile(t, parser.NewGithubParser(), ".github/workflows/ci.yml", workflow)
			assert.Equal(t, tc.expected, verdicts["Linter"])
		})
	}
}

// Harden-Runner watches the runner for the rest of the job it is in;
// egress-policy audit only records what happened, block (the default) refuses
// the calls that are not allowed.
func TestHardenRunnerEgressPolicyGradesCorrectly(t *testing.T) {
	for _, tc := range []struct {
		name     string
		input    string
		expected Verdict
	}{
		{name: "default egress-policy is block", expected: Enforcing},
		{name: "allowed-endpoints without egress-policy is enforcing", input: "allowed-endpoints: github.com:443", expected: Enforcing},
		{name: "egress-policy block is enforcing", input: "egress-policy: block", expected: Enforcing},
		{name: "egress-policy audit only records", input: "egress-policy: audit", expected: Advisory},
		{name: "expression is unknown", input: "egress-policy: ${{ inputs.policy }}", expected: Unknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			workflow := `on: pull_request
jobs:
  build:
    steps:
      - uses: step-security/harden-runner@v2
`
			if tc.input != "" {
				workflow += "        with:\n          " + tc.input + "\n"
			}
			verdicts := gradeFile(t, parser.NewGithubParser(), ".github/workflows/ci.yml", workflow)
			assert.Equal(t, tc.expected, verdicts["Workflow Security"])
		})
	}
}

// Cypress's action installs dependencies and runs the tests; with runTests
// off it only installs, builds and starts servers.
func TestCypressRunTestsInputGradesCorrectly(t *testing.T) {
	for _, tc := range []struct {
		name     string
		input    string
		expected Verdict
	}{
		{name: "default runs the tests", expected: Enforcing},
		{name: "runTests false only installs", input: "runTests: false", expected: Advisory},
		{name: "runTests 0 only installs", input: "runTests: 0", expected: Advisory},
		{name: "expression is unknown", input: "runTests: ${{ inputs.run }}", expected: Unknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			workflow := `on: pull_request
jobs:
  e2e:
    steps:
      - uses: cypress-io/github-action@v6
`
			if tc.input != "" {
				workflow += "        with:\n          " + tc.input + "\n"
			}
			verdicts := gradeFile(t, parser.NewGithubParser(), ".github/workflows/e2e.yml", workflow)
			assert.Equal(t, tc.expected, verdicts["Unit Tests"])
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


func TestCSpellNoExitCodeGradesAdvisory(t *testing.T) {
	for run, expected := range map[string]Verdict{
		"npx cspell lint .":                 Enforcing,
		"npx cspell lint --no-exit-code .": Advisory,
	} {
		verdicts := gradeFile(t, parser.NewGithubParser(), ".github/workflows/ci.yml", `
on: pull_request
jobs:
  spelling:
    steps:
      - run: `+run+`
`)
		assert.Equal(t, expected, verdicts["Linter"], run)
	}
}

func TestCSpellActionStrictInputGradesCorrectly(t *testing.T) {
	for _, tc := range []struct {
		name     string
		input    string
		expected Verdict
	}{
		{name: "default is enforcing", expected: Enforcing},
		{name: "strict false is advisory", input: "strict: false", expected: Advisory},
		{name: "expression is unknown", input: "strict: ${{ inputs.strict }}", expected: Unknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			workflow := `on: pull_request
jobs:
  spelling:
    steps:
      - uses: streetsidesoftware/cspell-action@v9
`
			if tc.input != "" {
				workflow += "        with:\n          " + tc.input + "\n"
			}
			verdicts := gradeFile(t, parser.NewGithubParser(), ".github/workflows/ci.yml", workflow)
			assert.Equal(t, tc.expected, verdicts["Linter"])
		})
	}
}

// github-action-benchmark only comments on a regression unless fail-on-alert
// is true, and its former name rhysd/ is still in older workflows.
func TestGithubActionBenchmarkFailOnAlertGradesAdvisory(t *testing.T) {
	for _, tc := range []struct {
		name     string
		uses     string
		input    string
		expected Verdict
	}{
		{name: "fail-on-alert true is a gate", uses: "benchmark-action/github-action-benchmark@v1", input: "fail-on-alert: true", expected: Enforcing},
		{name: "fail-on-alert default is advisory", uses: "benchmark-action/github-action-benchmark@v1", expected: Advisory},
		{name: "fail-on-alert false is advisory", uses: "benchmark-action/github-action-benchmark@v1", input: "fail-on-alert: false", expected: Advisory},
		{name: "former name with true is a gate", uses: "rhysd/github-action-benchmark@v1", input: "fail-on-alert: TRUE", expected: Enforcing},
	} {
		t.Run(tc.name, func(t *testing.T) {
			workflow := `on: pull_request
jobs:
  bench:
    steps:
      - uses: ` + tc.uses + `
`
			if tc.input != "" {
				workflow += "        with:\n          " + tc.input + "\n"
			}
			verdicts := gradeFile(t, parser.NewGithubParser(), ".github/workflows/ci.yml", workflow)
			assert.Equal(t, tc.expected, verdicts["Performance Testing"])
		})
	}
}

// Each of these tools reports and exits 0 in some form, or stops failing on
// part of what it finds.
func TestToolFormsThatCannotFail(t *testing.T) {
	step := func(lines ...string) string {
		return "    runs-on: ubuntu-latest\n    steps:\n      - " + strings.Join(lines, "\n        ") + "\n"
	}
	osv := "    uses: google/osv-scanner-action/.github/workflows/osv-scanner-reusable.yml@v2.3.0\n"
	for _, tt := range []struct {
		name, category, job string
		want                Verdict
	}{
		{"zizmor action default", "Workflow Security", step("uses: zizmorcore/zizmor-action@v0.8.1"), Advisory},
		{"zizmor action advanced security", "Workflow Security", step("uses: zizmorcore/zizmor-action@v0.8.1", "with:", "  advanced-security: true"), Advisory},
		{"zizmor action fails", "Workflow Security", step("uses: zizmorcore/zizmor-action@v0.8.1", "with:", "  advanced-security: 'false'"), Enforcing},
		{"zizmor action expression", "Workflow Security", step("uses: zizmorcore/zizmor-action@v0.8.1", "with:", "  advanced-security: ${{ inputs.sarif }}"), Unknown},
		{"zizmor sarif", "Workflow Security", step("run: uvx zizmor --format=sarif . > results.sarif"), Advisory},
		{"zizmor no exit codes", "Workflow Security", step("run: zizmor --no-exit-codes .github/workflows"), Advisory},
		{"zizmor", "Workflow Security", step("run: zizmor --format github --min-severity high ."), Enforcing},
		{"buf setup only", "Linter", step("uses: bufbuild/buf-action@v1", "with:", "  setup_only: true"), Advisory},
		{"buf action", "Linter", step("uses: bufbuild/buf-action@v1"), Enforcing},
		{"swiftlint fix", "Linter", step("run: docker run --rm -v $PWD:/work ghcr.io/realm/swiftlint:latest --fix"), Advisory},
		{"swiftlint lenient", "Linter", step("run: swiftlint lint --lenient"), Advisory},
		{"swiftlint strict", "Linter", step("run: swiftlint lint --strict"), Enforcing},
		{"flutter no fatal infos", "Linter", step("run: flutter analyze --no-fatal-infos"), Partial},
		{"dart no fatal warnings", "Linter", step("run: dart analyze --no-fatal-warnings lib"), Partial},
		{"dart fatal infos", "Linter", step("run: dart analyze --fatal-infos"), Enforcing},
		{"pre-commit staged files", "Linter", step("run: pre-commit run --show-diff-on-failure"), Advisory},
		{"pre-commit one hook", "Linter", step("uses: pre-commit/action@v3.0.1", "with:", "  extra_args: trailing-whitespace"), Advisory},
		{"pre-commit arguments", "Linter", step("run: pre-commit run $PRE_COMMIT_ARGS"), Unknown},
		{"pre-commit all files", "Linter", step("run: uv run pre-commit run -a"), Enforcing},
		{"pre-commit refs", "Linter", step("run: pre-commit run --from-ref origin/main --to-ref HEAD"), Enforcing},
		{"pre-commit action", "Linter", step("uses: pre-commit/action@v3.0.1"), Enforcing},
		{"prek last commit", "Linter", step("run: prek run --last-commit"), Enforcing},
		{"osv no fail", "SCA", osv + "    with:\n      fail-on-vuln: false\n", Advisory},
		{"osv", "SCA", osv, Enforcing},
		{"govulncheck sarif", "SCA", step("run: go run golang.org/x/vuln/cmd/govulncheck@v1.1.4 -format sarif ./... > gh.sarif"), Advisory},
		{"govulncheck json", "SCA", step("run: govulncheck -json ./..."), Advisory},
		{"govulncheck json off", "SCA", step("run: govulncheck -json=false ./..."), Enforcing},
		{"govulncheck text", "SCA", step("run: govulncheck -format text ./..."), Enforcing},
		{"govulncheck action sarif", "SCA", step("uses: golang/govulncheck-action@v1", "with:", "  output-format: sarif"), Advisory},
		{"govulncheck action", "SCA", step("uses: golang/govulncheck-action@v1"), Enforcing},
	} {
		t.Run(tt.name, func(t *testing.T) {
			workflow := "on: pull_request\njobs:\n  checks:\n" + tt.job
			verdicts := gradeFile(t, parser.NewGithubParser(), ".github/workflows/checks.yml", workflow)
			assert.Equal(t, tt.want, verdicts[tt.category])
		})
	}
}

// ty exits 1 on error-level diagnostics unless --exit-zero says otherwise;
// --exit-zero-on-warning still fails on errors.
func TestTyExitZeroGradesAdvisory(t *testing.T) {
	for run, expected := range map[string]Verdict{
		"uv run ty check":                 Enforcing,
		"uv run ty check --exit-zero":     Advisory,
		"ty check --exit-zero-on-warning": Enforcing,
	} {
		verdicts := gradeFile(t, parser.NewGithubParser(), ".github/workflows/ci.yml", `
on: pull_request
jobs:
  typecheck:
    steps:
      - run: `+run+`
`)
		assert.Equal(t, expected, verdicts["Linter"], run)
	}
}

// Invoke-Pester reports failures and sets $LASTEXITCODE without exiting. Only
// -CI and -EnableExit make the call itself exit non-zero; -CI:$false turns
// -CI off, and any other invocation leaves the outcome to what runs around
// it, which the workflow text alone cannot say.
func TestPesterExitFlagsGradeCorrectly(t *testing.T) {
	for run, expected := range map[string]Verdict{
		"Invoke-Pester -Path tests -CI":                   Enforcing,
		"Invoke-Pester -Script ./tests -EnableExit":       Enforcing,
		"pwsh -Command \"Invoke-Pester -Path tests -CI\"": Enforcing,
		"pwsh -c 'Invoke-Pester -CI'":                     Enforcing,
		"Invoke-Pester -Path tests -Output Detailed":      Unknown,
		"Invoke-Pester -Configuration $config":            Unknown,
		"Invoke-Pester -Path tests -CI:$false":            Unknown,
	} {
		verdicts := gradeFile(t, parser.NewGithubParser(), ".github/workflows/ci.yml", `
on: pull_request
jobs:
  pwsh:
    steps:
      - run: `+run+`
`)
		assert.Equal(t, expected, verdicts["Unit Tests"], run)
	}
}
