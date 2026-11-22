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
		{"quoted opengrep scan", `"./bin/opengrep" scan .`, "SAST", Advisory},
		{"quoted opengrep error", `"./bin/opengrep" scan --error .`, "SAST", Enforcing},
		{"opengrep ci", "opengrep ci", "SAST", Enforcing},
		{"opengrep global flag ci", `"$RUNNER_TEMP/opengrep" --debug ci`, "SAST", Enforcing},
		{"opengrep scan without subcommand", "opengrep --config auto .", "SAST", Advisory},
		{"opengrep scan with no-error", "opengrep scan --no-error .", "SAST", Advisory},
		{"opengrep wrapper script", "scripts/run-opengrep.sh --changed --sarif --error", "SAST", Enforcing},
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
// fail.
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

// lockfile-lint checks nothing unless a validator is enabled; a bare command
// may get its path and validators from configuration that is not read here.
func TestLockfileLintExitsZeroUnlessValidator(t *testing.T) {
	for _, tt := range []struct {
		name, run string
		want      Verdict
	}{
		{"https", "npx lockfile-lint --path package-lock.json --validate-https", Enforcing},
		{"hosts", "npx lockfile-lint --path yarn.lock --allowed-hosts npm yarn", Enforcing},
		{"schemes", `npx lockfile-lint --path yarn.lock --allowed-schemes "https:" "git+https:"`, Enforcing},
		{"urls", "npx lockfile-lint --path yarn.lock --allowed-urls https://example.com/package.tgz", Enforcing},
		{"integrity", "npx lockfile-lint --path yarn.lock --validate-integrity", Enforcing},
		{"strict integrity", "npx lockfile-lint --path yarn.lock --validate-integrity-strict", Enforcing},
		{"package names", "npx lockfile-lint --path yarn.lock --validate-package-names", Enforcing},
		{"short https", "npx lockfile-lint -p yarn.lock -s", Enforcing},
		{"short hosts", "npx lockfile-lint -p yarn.lock -a npm yarn", Enforcing},
		{"short schemes", "npx lockfile-lint -p yarn.lock -o https:", Enforcing},
		{"short urls", "npx lockfile-lint -p yarn.lock -u https://example.com/package.tgz", Enforcing},
		{"short integrity", "npx lockfile-lint -p yarn.lock -i", Enforcing},
		{"short package names", "npx lockfile-lint -p yarn.lock -n", Enforcing},
		{"long equals", "npx lockfile-lint --path yarn.lock --allowed-hosts=npm", Enforcing},
		{"boolean equals", "npx lockfile-lint --path yarn.lock --validate-https=true", Enforcing},
		{"short equals", "npx lockfile-lint -p yarn.lock -a=npm", Enforcing},
		{"versioned runner", "npx --yes lockfile-lint@5 --path package-lock.json --type npm --validate-https --allowed-hosts npm", Enforcing},
		{"short validators", "npx lockfile-lint -p yarn.lock -s -a npm yarn", Enforcing},
		{"npm exec", "npm exec --no-install lockfile-lint -- --type npm --path package-lock.json --validate-https", Enforcing},
		{"npx offline", "npx --offline lockfile-lint --path yarn.lock -s", Enforcing},
		{"npx prefer offline", "npx --prefer-offline lockfile-lint --path yarn.lock -s", Enforcing},
		{"yarn offline", "yarn --offline lockfile-lint --path yarn.lock -s", Enforcing},
		{"bun runtime", "bunx --bun lockfile-lint --path yarn.lock -s", Enforcing},
		{"package with other runner options", "npx --package lockfile-lint@5 --prefer-offline -s lockfile-lint --path yarn.lock --validate-https", Enforcing},
		{"quoted package path", `npx --package "/tmp/tool cache/lockfile-lint" --offline lockfile-lint --path yarn.lock -s`, Enforcing},
		{"no validator", "npx lockfile-lint --path package-lock.json --type npm", Advisory},
		{"format only", "npx lockfile-lint --path package-lock.json --format plain", Advisory},
		{"empty hostname only", "npx lockfile-lint --path package-lock.json --empty-hostname false", Advisory},
		{"package aliases only", "npx lockfile-lint --path package-lock.json --allowed-package-name-aliases example", Advisory},
		{"package alias named like tool", "npx lockfile-lint --path yarn.lock --allowed-package-name-aliases lockfile-lint", Advisory},
		{"path named like tool", "npx lockfile-lint --path lockfile-lint", Advisory},
		{"yarn silent", "yarn -s lockfile-lint --path yarn.lock", Advisory},
		{"pnpm silent", "pnpm -s lockfile-lint --path yarn.lock", Advisory},
		{"runner flags before binary", "npx --yes --package=lockfile-lint@5.0.1 lockfile-lint --path package-lock.json", Advisory},
		{"package value before validator", "npx --package lockfile-lint@5 -s lockfile-lint --path yarn.lock --validate-https", Enforcing},
		{"short package value before validator", "npx -p lockfile-lint@5 -s lockfile-lint --path yarn.lock --validate-https", Enforcing},
		{"inline environment", "CI=true npx lockfile-lint --path yarn.lock -s", Enforcing},
		{"sudo", "sudo lockfile-lint --path yarn.lock -s", Enforcing},
		{"long flag suffix", "npx lockfile-lint --path yarn.lock --validate-https-extra", Advisory},
		{"short flag suffix", "npx lockfile-lint --path yarn.lock -something", Advisory},
		{"flag case", "npx lockfile-lint --path yarn.lock --VALIDATE-HTTPS", Advisory},
		{"bare", "lockfile-lint", Unknown},
		{"bare runner", "npx lockfile-lint", Unknown},
		{"bare versioned runner", "npx --yes lockfile-lint@5", Unknown},
		{"quoted binary", `"$RUNNER_TEMP/promptfoo-ci-tools/node_modules/.bin/lockfile-lint" --path yarn.lock --validate-https`, Enforcing},
		{"continued command", "npx lockfile-lint \\\n  --path package-lock.json \\\n  --validate-https", Enforcing},
		{"neighbor flag", "echo --validate-https\nnpx lockfile-lint --path package-lock.json", Advisory},
		{"following flag", "npx lockfile-lint --path package-lock.json; echo --validate-https", Advisory},
		{"one validator suffices", "npx lockfile-lint --path package-lock.json\nnpx lockfile-lint --path yarn.lock --validate-https", Enforcing},
		{"swallowed validation", "npx lockfile-lint --path package-lock.json --validate-https || true", Advisory},
	} {
		t.Run(tt.name, func(t *testing.T) {
			verdicts := gradeFile(t, parser.NewGithubParser(), ".github/workflows/ci.yml", `
on: pull_request
jobs:
  lockfile:
    steps:
      - run: |
          `+strings.ReplaceAll(tt.run, "\n", "\n          ")+`
`)
			assert.Equal(t, tt.want, verdicts["Supply Chain"])
			assert.NotContains(t, verdicts, "SCA")
		})
	}
}

func TestLockfileLintPackageOptionIsNotAnInvocation(t *testing.T) {
	for _, run := range []string{
		"npx --package lockfile-lint@5 other-tool",
		`npx --package "/tmp/tools/lockfile-lint@5" -s other-tool`,
		`npx --package "other lockfile-lint@5" -s other-tool`,
	} {
		verdicts := gradeFile(t, parser.NewGithubParser(), ".github/workflows/ci.yml", `
on: pull_request
jobs:
  check:
    steps:
      - run: `+run+`
`)
		assert.NotContains(t, verdicts, "Supply Chain", run)
	}
}

func TestLockfileLintFoldedCommand(t *testing.T) {
	verdicts := gradeFile(t, parser.NewGithubParser(), ".github/workflows/ci.yml", `
on: pull_request
jobs:
  lockfile:
    steps:
      - run: >
          npx --yes lockfile-lint@5
          --path package-lock.json --type npm
          --validate-https --allowed-hosts npm
`)
	assert.Equal(t, Enforcing, verdicts["Supply Chain"])
	assert.NotContains(t, verdicts, "SCA")
}

func TestLockfileLintContinueOnErrorGradesAdvisory(t *testing.T) {
	verdicts := gradeFile(t, parser.NewGithubParser(), ".github/workflows/ci.yml", `
on: pull_request
jobs:
  lockfile:
    steps:
      - run: npx lockfile-lint --path package-lock.json --validate-https
        continue-on-error: true
`)
	assert.Equal(t, Advisory, verdicts["Supply Chain"])
	assert.NotContains(t, verdicts, "SCA")
}

// Asking a linter for its version through a package runner does not run it (#607).
func TestRunnerVersionCheckIsNotALinterRun(t *testing.T) {
	grade := func(run string) map[string]Verdict {
		return gradeFile(t, parser.NewGithubParser(), ".github/workflows/ci.yml", `
on: pull_request
jobs:
  lint:
    steps:
      - run: `+run+`
`)
	}
	assert.NotContains(t, grade("npx eslint --version"), "Linter")
	assert.Contains(t, grade("npx eslint ."), "Linter")
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

// Sobelow exits successfully on findings unless --exit is present, and
// --config may change the exit behavior via .sobelow-conf.
func TestSobelowExitFlagControlsGating(t *testing.T) {
	for run, expected := range map[string]Verdict{
		"mix sobelow":               Advisory,
		"mix sobelow --exit":        Enforcing,
		"mix sobelow --exit medium": Enforcing,
		"mix sobelow --skip --exit": Enforcing,
		"mix sobelow --config":      Unknown,
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

// sobelow/action passes its flags input to the same scanning command.
func TestSobelowActionFlagsControlGating(t *testing.T) {
	for flags, expected := range map[string]Verdict{
		"":                 Advisory,
		"-i Config":        Advisory,
		"-i Config --exit": Enforcing,
		"--config":         Unknown,
	} {
		verdicts := gradeFile(t, parser.NewGithubParser(), ".github/workflows/ci.yml", `
on: pull_request
jobs:
  scan:
    steps:
      - uses: sobelow/action@v1
        with:
          flags: "`+flags+`"
`)
		assert.Equal(t, expected, verdicts["SAST"], flags)
	}
}

// HTMLHint fails on findings by default; --warn keeps the linter but makes it
// advisory because the command exits successfully.
func TestHTMLHintWarnGradesAdvisory(t *testing.T) {
	for run, expected := range map[string]Verdict{
		`npx htmlhint "src" --config .htmlhintrc`: Enforcing,
		`npx htmlhint . --format sarif`:           Enforcing,
		`npx htmlhint --warn "src"`:               Advisory,
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

func TestReraisedFailureGradesEnforcing(t *testing.T) {
	for raise, expected := range map[string]Verdict{
		"run: exit 1":                  Enforcing,
		"run: gh issue create -t vuln": Advisory,
	} {
		verdicts := gradeFile(t, parser.NewGithubParser(), ".github/workflows/ci.yml", `
on: pull_request
jobs:
  scan:
    steps:
      - id: trivy
        uses: aquasecurity/trivy-action@0.28.0
        continue-on-error: true
        with:
          exit-code: "1"
      - if: steps.trivy.outcome == 'failure'
        `+raise+`
`)
		assert.Equal(t, expected, verdicts["SCA"], raise)
	}
}

// OUT=$(tool) fails with the tool under bash -e; anything else around the
// substitution hides its exit status.
func TestCommandSubstitutionGradesByWhatKeepsTheStatus(t *testing.T) {
	for run, expected := range map[string]Verdict{
		"OUT=$(gitleaks detect --source . 2>&1)":    Enforcing,
		"OUT=$(gitleaks detect --source .) || true": Advisory,
		"export OUT=$(gitleaks detect --source .)":  Advisory,
		`echo "$(gitleaks detect --source .)"`:      Advisory,
	} {
		verdicts := gradeFile(t, parser.NewGithubParser(), ".github/workflows/ci.yml", `
on: pull_request
jobs:
  scan:
    steps:
      - run: |
          `+run+`
`)
		assert.Equal(t, expected, verdicts["Secrets Detection"], run)
	}
}

// With set +e a scan that is not the last command fails nothing unless its
// $? is read, and a case that lets its findings code through only reports.
func TestSetPlusEGradesLostFailuresAdvisory(t *testing.T) {
	for run, expected := range map[string]Verdict{
		"set +e\n          gitleaks detect --source .\n          echo done":                                                        Advisory,
		"set +e\n          gitleaks detect --source .\n          rc=$?\n          exit $rc":                                        Enforcing,
		"set +e\n          gitleaks detect --source .\n          rc=$?\n          case $rc in 0|1) echo ok ;; *) exit $rc ;; esac": Advisory,
	} {
		verdicts := gradeFile(t, parser.NewGithubParser(), ".github/workflows/ci.yml", `
on: pull_request
jobs:
  scan:
    steps:
      - run: |
          `+run+`
`)
		assert.Equal(t, expected, verdicts["Secrets Detection"], run)
	}
}

// Secretlint does not fail on findings when writing its report with --output.
func TestSecretlintOutputGradesAdvisory(t *testing.T) {
	for run, expected := range map[string]Verdict{
		`npx secretlint "**/*"`:                                          Enforcing,
		"secretlint --format sarif .":                                    Enforcing,
		`npx secretlint "**/*" --format sarif --output secretlint.sarif`: Advisory,
		"secretlint --output=report.json .":                              Advisory,
		"secretlint --output-format sarif .":                             Enforcing,
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

// Talisman scans fail on findings, but its pre-commit hook only examines
// staged changes; --checksum merely lists .talismanrc entries.
func TestTalismanRunGrading(t *testing.T) {
	for _, tt := range []struct {
		name, run string
		want      Verdict
	}{
		{"scan", "talisman --scan", Enforcing},
		{"scan with path", "./talisman -i . --scan", Enforcing},
		{"pre-commit long flag", "./talisman --githook pre-commit", Partial},
		{"pre-commit short flag", "talisman -g pre-commit", Partial},
		{"pre-push hook", `echo "$LOCAL_REF $LOCAL_SHA $REMOTE_REF $REMOTE_SHA" | ./talisman --githook pre-push`, Enforcing},
		{"checksum", "talisman --checksum 'config/*'", ""},
		{"checksum short flag", "talisman -c=config/*", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			verdicts := gradeFile(t, parser.NewGithubParser(), ".github/workflows/talisman.yml", `
on: pull_request
jobs:
  scan:
    steps:
      - run: `+tt.run+`
`)
			if tt.want == "" {
				assert.NotContains(t, verdicts, "Secrets Detection")
			} else {
				assert.Equal(t, tt.want, verdicts["Secrets Detection"])
			}
		})
	}
}

// kubeconform and kubeval fail on an invalid manifest, and no flag stops
// that; only a swallowed exit status makes them advisory.
func TestKubeconformKubevalGradesEnforcing(t *testing.T) {
	for run, expected := range map[string]Verdict{
		"helm template ci/helm-chart | kubeconform -strict -summary -": Enforcing,
		"kubeconform -strict -summary -output json manifests/":         Enforcing,
		"kubeconform -strict -ignore-missing-schemas manifests/":       Enforcing,
		"kubeconform -strict manifests/ || true":                       Advisory,
		"kubeval --strict -d k8s/":                                     Enforcing,
	} {
		t.Run(run, func(t *testing.T) {
			verdicts := gradeFile(t, parser.NewGithubParser(), ".github/workflows/ci.yml", `
on: pull_request
jobs:
  validate:
    steps:
      - run: `+run+`
`)
			assert.Equal(t, expected, verdicts["IaC Security"])
		})
	}
}

func TestMarkdownLinkCheckGradesEnforcing(t *testing.T) {
	for _, tt := range []struct {
		name, step string
		want       Verdict
	}{
		{"command", "run: markdown-link-check ./docs", Enforcing},
		{"npx with config", "run: npx markdown-link-check -c .mlc.json README.md", Enforcing},
		{"swallowed exit status", "run: markdown-link-check ./docs || true", Advisory},
		{"tcort quiet", "uses: tcort/github-action-markdown-link-check@v1\n        with:\n          use-quiet-mode: yes", Enforcing},
		{"tcort may fail", "uses: tcort/github-action-markdown-link-check@v1\n        continue-on-error: true\n        with:\n          use-quiet-mode: yes", Advisory},
		{"gaurav-nelson quiet", "uses: gaurav-nelson/github-action-markdown-link-check@v1\n        with:\n          use-quiet-mode: yes", Enforcing},
		{"gaurav-nelson may fail", "uses: gaurav-nelson/github-action-markdown-link-check@v1\n        continue-on-error: true\n        with:\n          use-quiet-mode: yes", Advisory},
	} {
		t.Run(tt.name, func(t *testing.T) {
			verdicts := gradeFile(t, parser.NewGithubParser(), ".github/workflows/ci.yml", `
on: pull_request
jobs:
  links:
    steps:
      - `+tt.step+`
`)
			assert.Equal(t, tt.want, verdicts["Linter"])
		})
	}
}

func TestMarkdownLinkCheckNamesAndPassiveCommandsDoNotCoverLinter(t *testing.T) {
	for _, tt := range []struct {
		name, workflowName, job, run string
	}{
		{"workflow name", "markdown-link-check", "checks", "echo unrelated"},
		{"job name", "CI", "markdown-link-check", "echo unrelated"},
		{"install", "CI", "checks", "npm install -g markdown-link-check@3.8.7"},
		{"sudo install", "CI", "checks", "sudo npm install -g markdown-link-check@3.8.7"},
		{"version", "CI", "checks", "markdown-link-check --version"},
		{"help", "CI", "checks", "markdown-link-check --help"},
		{"npx help", "CI", "checks", "npx --yes markdown-link-check --help"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			verdicts := gradeFile(t, parser.NewGithubParser(), ".github/workflows/ci.yml", `
name: `+tt.workflowName+`
on: pull_request
jobs:
  `+tt.job+`:
    steps:
      - run: `+tt.run+`
`)
			assert.NotContains(t, verdicts, "Linter")
		})
	}
}

func TestTalismanActionsGradeEnforcing(t *testing.T) {
	for _, action := range []string{
		"carhartl/talisman-secrets-scan-action@v1.4.0",
		"digitalservicebund/talisman-secrets-scan-action@0123456789abcdef0123456789abcdef01234567",
	} {
		t.Run(action, func(t *testing.T) {
			verdicts := gradeFile(t, parser.NewGithubParser(), ".github/workflows/talisman.yml", `
on: pull_request
jobs:
  scan:
    steps:
      - uses: `+action+`
`)
			assert.Equal(t, Enforcing, verdicts["Secrets Detection"])
		})
	}
}

func TestPolarisAuditGradesExitCodeSettings(t *testing.T) {
	for run, expected := range map[string]Verdict{
		"polaris audit --audit-path deploy/":                                Advisory,
		"polaris audit --set-exit-code-on-danger --audit-path deploy/":      Enforcing,
		"polaris audit --set-exit-code-on-danger=true":                      Enforcing,
		"polaris audit --set-exit-code-below-score=90":                      Enforcing,
		"polaris audit --set-exit-code-on-danger=false":                     Advisory,
		"polaris audit --set-exit-code-below-score ${{ inputs.min_score }}": Unknown,
	} {
		t.Run(run, func(t *testing.T) {
			verdicts := gradeFile(t, parser.NewGithubParser(), ".github/workflows/polaris.yml", `
on: pull_request
jobs:
  audit:
    steps:
      - run: `+run+`
`)
			assert.Equal(t, expected, verdicts["IaC Security"], run)
		})
	}
}

// The kubeconform image run as a docker:// step enforces unless the step
// may fail.
func TestKubeconformDockerStepGradesEnforcing(t *testing.T) {
	for extra, expected := range map[string]Verdict{
		"":                                  Enforcing,
		"\n        continue-on-error: true": Advisory,
	} {
		verdicts := gradeFile(t, parser.NewGithubParser(), ".github/workflows/kubeconform.yml", `
on: pull_request
jobs:
  validate:
    steps:
      - uses: docker://ghcr.io/yannh/kubeconform:v0.8.0
        with:
          args: -strict k8s/`+extra+`
`)
		assert.Equal(t, expected, verdicts["IaC Security"], extra)
	}
}

func TestSQLFluffNoFailGradesAdvisory(t *testing.T) {
	for run, expected := range map[string]Verdict{
		"sqlfluff lint models/":                             Enforcing,
		"sqlfluff lint models/ --nofail":                    Advisory,
		"python -m sqlfluff lint --nofail --dialect ansi .": Advisory,
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

func TestVultureGradesCorrectly(t *testing.T) {
	for run, expected := range map[string]Verdict{
		"vulture src tests --min-confidence 80":           Enforcing,
		"uv run vulture --min-confidence 100 .":           Enforcing,
		"vulture --min-confidence 100 apifuzzer/ || true": Advisory,
	} {
		t.Run(run, func(t *testing.T) {
			verdicts := gradeFile(t, parser.NewGithubParser(), ".github/workflows/ci.yml", `
on: pull_request
jobs:
  lint:
    steps:
      - run: `+run+`
`)
			assert.Equal(t, expected, verdicts["Linter"], run)
		})
	}
}

func TestInterrogateGradesCorrectly(t *testing.T) {
	for run, expected := range map[string]Verdict{
		"interrogate -v --fail-under 80 mmengine":                       Enforcing,
		"uv run --with interrogate interrogate --config=pyproject.toml": Enforcing,
		"pipx run interrogate -v .":                                     Enforcing,
		"interrogate -f 33.0 sycret":                                    Enforcing,
		"interrogate --fail-under 0.5 src":                              Enforcing,
		"interrogate --fail-under 0 src":                                Advisory,
		"interrogate --fail-under=0 src":                                Advisory,
		"interrogate -f 0 src":                                          Advisory,
		"interrogate -f0 src":                                           Advisory,
		"interrogate --fail-under 0.0 src":                              Advisory,
		"interrogate huixiangdou -v || true":                            Advisory,
	} {
		t.Run(run, func(t *testing.T) {
			verdicts := gradeFile(t, parser.NewGithubParser(), ".github/workflows/ci.yml", `
on: pull_request
jobs:
  lint:
    steps:
      - run: `+run+`
`)
			assert.Equal(t, expected, verdicts["Linter"], run)
		})
	}
}

func TestInterrogateActionFailUnderGradesCorrectly(t *testing.T) {
	for _, tc := range []struct {
		name     string
		input    string
		expected Verdict
	}{
		{name: "default is enforcing", expected: Enforcing},
		{name: "fail-under 0 is advisory", input: "fail-under: '0'", expected: Advisory},
		{name: "fail-under 95 is enforcing", input: "fail-under: 95", expected: Enforcing},
		{name: "expression is unknown", input: "fail-under: ${{ inputs.fail-under }}", expected: Unknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			workflow := `on: pull_request
jobs:
  docstrings:
    steps:
      - uses: JackMcKew/python-interrogate-check@main
        with:
          path: braces
`
			if tc.input != "" {
				workflow += "          " + tc.input + "\n"
			}
			verdicts := gradeFile(t, parser.NewGithubParser(), ".github/workflows/ci.yml", workflow)
			assert.Equal(t, tc.expected, verdicts["Linter"])
		})
	}
}

// Scalafix's --check fails the run on a pending rewrite. Without it rewrites
// are applied and only a linter rule in .scalafix.conf can fail the run, which
// the command doesn't show, so it is unknown.
func TestScalafixCheckGradesCorrectly(t *testing.T) {
	for run, expected := range map[string]Verdict{
		`sbt "scalafixAll --check"`:                                Enforcing,
		`sbt 'rootJVM/scalafixAll --check'`:                        Enforcing,
		`sbt "Test / scalafix --test"`:                             Enforcing,
		"sbt scalafixCheckAll":                                     Enforcing,
		"./mill __.fix --check":                                    Enforcing,
		"./mill -i __.fix --check":                                 Enforcing,
		"scalafix --check --syntactic src":                         Enforcing,
		"cs launch scalafix -- --check":                            Enforcing,
		`sbt "scalafixAll --check" || true`:                        Advisory,
		"sbt scalafixAll":                                          Unknown,
		`sbt "scalafixAll RemoveUnused"`:                           Unknown,
		`sbt "scalafixAll" "test --check"`:                         Unknown,
		"./mill __.fix":                                            Unknown,
		"./mill __.fix + __.test --check":                          Unknown,
		"scalafix --rules OrganizeImports":                         Unknown,
		"./mill 'core[2.13.16].fix' --check":                       Enforcing,
		`./mill "__.fix" --check`:                                  Enforcing,
		"|\n          sbt \\\n            \"scalafixAll --check\"": Enforcing,
		"|\n          cd scalafix\n          sbt test":             "",
		"cd modules/scalafix && sbt test":                          "",
		"git clone https://github.com/scalacenter/scalafix":        "",
		"./mill __.test | tee scalafix":                            "",
		"mvn -pl mill fix":                                         "",
		"make -C mill fix":                                         "",
		"./mill __.compile fix":                                    "",
		"./mill resolve __.fix":                                    "",
		"./mill __.test 2>&1 > fix":                                "",
		`'nix develop .#${{ matrix.java }} -c sbt ''project ${{ matrix.project }}'' ''++ ${{ matrix.scala }}'' ''scalafixAll --check'''`: Enforcing,
		`timeout 30m sbt "scalafixAll --check"`:    Enforcing,
		"cs launch scalafix:0.14.3 -- --check src": Enforcing,
		"./mill -j 4 __.fix --check":               Enforcing,
	} {
		t.Run(run, func(t *testing.T) {
			verdicts := gradeFile(t, parser.NewGithubParser(), ".github/workflows/ci.yml", `
on: pull_request
jobs:
  lint:
    steps:
      - run: `+run+`
`)
			assert.Equal(t, expected, verdicts["Linter"], run)
		})
	}
}

// awesome_bot exits 1 on a dead or duplicate link; no flag makes it exit 0.
func TestAwesomeBotGradesCorrectly(t *testing.T) {
	for run, expected := range map[string]Verdict{
		"awesome_bot README.md --allow-dupe --allow-redirect": Enforcing,
		"awesome_bot README.md --skip-save-results":           Enforcing,
		"awesome_bot README.md --allow-ssl || true":           Advisory,
	} {
		t.Run(run, func(t *testing.T) {
			verdicts := gradeFile(t, parser.NewGithubParser(), ".github/workflows/ci.yml", `
on: pull_request
jobs:
  links:
    steps:
      - run: `+run+`
`)
			assert.Equal(t, expected, verdicts["Linter"], run)
		})
	}

	for extra, expected := range map[string]Verdict{
		"":                                  Enforcing,
		"\n        continue-on-error: true": Advisory,
	} {
		verdicts := gradeFile(t, parser.NewGithubParser(), ".github/workflows/ci.yml", `
on: pull_request
jobs:
  links:
    steps:
      - uses: docker://dkhamsing/awesome_bot:1.20.0
        with:
          args: --allow-dupe README.md`+extra+`
`)
		assert.Equal(t, expected, verdicts["Linter"], extra)
	}
}

func TestVultureActionGradesEnforcing(t *testing.T) {
	for _, with := range []string{
		"",
		"\n        with:\n          args: '--min-confidence 80'",
	} {
		t.Run(with, func(t *testing.T) {
			verdicts := gradeFile(t, parser.NewGithubParser(), ".github/workflows/ci.yml", `
on: pull_request
jobs:
  lint:
    steps:
      - uses: gtkacz/vulture-action@1.0.0`+with+`
`)
			assert.Equal(t, Enforcing, verdicts["Linter"])
		})
	}
}

func TestLegacyLinterActionsGradeEnforcing(t *testing.T) {
	for _, uses := range []string{
		"github/super-linter/slim@v4",
		"github/super-linter@v4.9.7",
		"megalinter/megalinter/flavors/go@v8.4.2",
		"nvuillam/mega-linter@v4",
		"docker://ghcr.io/github/super-linter:slim-v4",
	} {
		t.Run(uses, func(t *testing.T) {
			verdicts := gradeFile(t, parser.NewGithubParser(), ".github/workflows/ci.yml", `
on: pull_request
jobs:
  lint:
    steps:
      - uses: `+uses+`
`)
			assert.Equal(t, Enforcing, verdicts["Linter"])
		})
	}
}

func TestLegacyLinterShellSteps(t *testing.T) {
	t.Run("Docker invocation grades enforcing", func(t *testing.T) {
		verdicts := gradeFile(t, parser.NewGithubParser(), ".github/workflows/ci.yml", `
on: pull_request
jobs:
  lint:
    steps:
      - run: docker run --env-file ".github/super-linter.env" ghcr.io/github/super-linter:slim-v4
`)
		assert.Equal(t, Enforcing, verdicts["Linter"])
	})

	t.Run("config-only step has no linter verdict", func(t *testing.T) {
		verdicts := gradeFile(t, parser.NewGithubParser(), ".github/workflows/ci.yml", `
on: pull_request
jobs:
  lint:
    steps:
      - run: grep -v '^#' .github/super-linter.env >> "$GITHUB_ENV"
`)
		assert.NotContains(t, verdicts, "Linter")
	})
}

// cargo-machete exits 1 on findings, and --fix still reports what it found.
func TestCargoMacheteGradesCorrectly(t *testing.T) {
	for run, expected := range map[string]Verdict{
		"cargo machete":       Enforcing,
		"cargo machete --fix": Enforcing,
		"cargo machete --with-metadata --skip-target-dir": Enforcing,
		"cargo-machete --json":                            Enforcing,
		"cargo machete --fix || true":                     Advisory,
	} {
		t.Run(run, func(t *testing.T) {
			verdicts := gradeFile(t, parser.NewGithubParser(), ".github/workflows/ci.yml", `
on: pull_request
jobs:
  lint:
    steps:
      - run: `+run+`
`)
			assert.Equal(t, expected, verdicts["Linter"], run)
		})
	}
}

// cargo-udeps exits 1 on findings and errors on a stable toolchain.
func TestCargoUdepsGradesCorrectly(t *testing.T) {
	for run, expected := range map[string]Verdict{
		"cargo +nightly udeps --all-targets":      Enforcing,
		"cargo udeps --all-targets --output json": Enforcing,
		"cargo udeps --all-targets || true":       Advisory,
	} {
		t.Run(run, func(t *testing.T) {
			verdicts := gradeFile(t, parser.NewGithubParser(), ".github/workflows/ci.yml", `
on: pull_request
jobs:
  lint:
    steps:
      - run: `+run+`
`)
			assert.Equal(t, expected, verdicts["Linter"], run)
		})
	}
}

// The cargo-machete action runs the binary, which fails on findings.
func TestCargoMacheteActionGradesCorrectly(t *testing.T) {
	for with, expected := range map[string]Verdict{
		"": Enforcing,
		"\n        with:\n          args: --fix --with-metadata": Enforcing,
		"\n        continue-on-error: true":                      Advisory,
	} {
		verdicts := gradeFile(t, parser.NewGithubParser(), ".github/workflows/cargo-machete.yml", `
on: pull_request
jobs:
  lint:
    steps:
      - uses: bnjbvr/cargo-machete@v0.9.2`+with+`
`)
		assert.Equal(t, expected, verdicts["Linter"], with)
	}
}

func TestGradleWrapperValidationGradesEnforcing(t *testing.T) {
	for _, tc := range []struct {
		name, action, inputs string
	}{
		{"current", "gradle/actions/wrapper-validation@v6", ""},
		{"minimum-zero", "gradle/actions/wrapper-validation@v6", "\n        with:\n          min-wrapper-count: 0"},
		{"snapshots", "gradle/actions/wrapper-validation@v6", "\n        with:\n          allow-snapshots: true"},
		{"allow-checksums", "gradle/actions/wrapper-validation@v6", "\n        with:\n          allow-checksums: '0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef'"},
		{"legacy", "gradle/wrapper-validation-action@v1", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			verdicts := gradeFile(t, parser.NewGithubParser(), ".github/workflows/ci.yml", `
on: pull_request
jobs:
  wrapper:
    steps:
      - uses: `+tc.action+tc.inputs+`
`)
			assert.Equal(t, Enforcing, verdicts["Supply Chain"])
		})
	}
}

// gh-action-pypi-publish attests only through trusted publishing to PyPI.
func TestPypiPublishAttestsOnlyThroughTrustedPublishing(t *testing.T) {
	for step, attests := range map[string]bool{
		"uses: pypa/gh-action-pypi-publish@release/v1":                                                                         true,
		"uses: pypa/gh-action-pypi-publish@release/v1\n        with:\n          attestations: false":                           false,
		"uses: pypa/gh-action-pypi-publish@release/v1\n        with:\n          password: ${{ secrets.PYPI }}":                 false,
		"uses: pypa/gh-action-pypi-publish@release/v1\n        with:\n          repository-url: https://test.pypi.org/legacy/": false,
		"uses: sigstore/gh-action-sigstore-python@v3.5.0\n        with:\n          inputs: ./dist/*.whl":                       true,
		"run: python -m sigstore sign dist/*.whl":                                                                              true,
	} {
		t.Run(step, func(t *testing.T) {
			verdicts := gradeFile(t, parser.NewGithubParser(), ".github/workflows/publish.yml", `
on: release
jobs:
  publish:
    steps:
      - `+step+`
`)
			if attests {
				assert.Equal(t, Enforcing, verdicts["Supply Chain"])
			} else {
				assert.NotContains(t, verdicts, "Supply Chain")
			}
		})
	}
}

// br hunt exits 10 on a listed package, and the Action fails the job then
// whatever fail-on or mode says; only continue-on-error or || true weaken it.
func TestBlastradiusGradesEnforcing(t *testing.T) {
	for step, expected := range map[string]Verdict{
		"run: br hunt npm:evil-dep@1.2.3 --path . --fail-if found":                                       Enforcing,
		"run: npx -y @jondot/blastradius hunt GHSA-xxxx-xxxx-xxxx --path .":                              Enforcing,
		"run: br hunt npm:evil-dep@1.2.3 --path . || true":                                               Advisory,
		"uses: jondot/blastradius@v1\n        with:\n          indicators: npm:evil-dep@1.2.3":           Enforcing,
		"uses: jondot/blastradius@v1\n        with:\n          fail-on: found\n          mode: advisory": Enforcing,
		"uses: jondot/blastradius@v1\n        continue-on-error: true":                                   Advisory,
	} {
		t.Run(step, func(t *testing.T) {
			verdicts := gradeFile(t, parser.NewGithubParser(), ".github/workflows/ci.yml", `
on: pull_request
jobs:
  scan:
    steps:
      - `+step+`
`)
			assert.Equal(t, expected, verdicts["SCA"])
		})
	}
}

// stroq inspect and its Action exit 1 on findings unless fail-on is none; vet never fails on them.
func TestStroqGradesByFailOn(t *testing.T) {
	for step, expected := range map[string]Verdict{
		"run: npx stroq inspect --sarif .":                                                  Enforcing,
		"run: npx @stroq/cli@0.32.0 inspect --changed-from HEAD^1 --format github .":        Enforcing,
		"run: npx stroq inspect --sarif . --fail-on none":                                   Advisory,
		"run: npx stroq inspect --sarif . > stroq.sarif || [ $? -eq 1 ]":                    Advisory,
		"run: npx @stroq/cli vet --from .claude/skills/deploy":                              Advisory,
		"uses: AGGIB/stroq-action@v1":                                                       Enforcing,
		"uses: AGGIB/stroq-action@v1\n        with:\n          fail-on: none":               Advisory,
		"uses: AGGIB/stroq-action@v1\n        with:\n          fail-on: ${{ inputs.mode }}": Unknown,
	} {
		t.Run(step, func(t *testing.T) {
			verdicts := gradeFile(t, parser.NewGithubParser(), ".github/workflows/ci.yml", `
on: pull_request
jobs:
  scan:
    steps:
      - `+step+`
`)
			assert.Equal(t, expected, verdicts["Agent Skill Security"])
		})
	}
}

// A step is weakened only when every run of the tool in it is. kolide/launcher
// gates on the first govulncheck and re-runs it with -format json to print.
func TestWeakenedOnlyWhenEveryRunIs(t *testing.T) {
	for run, expected := range map[string]Verdict{
		"if govulncheck -C . ./...; then\n  exit 0\nfi\ngovulncheck -C . -format json ./... > govulncheck.json\nexit 1": Enforcing,
		"govulncheck -format sarif ./... > a.sarif\ngovulncheck -json ./... > b.json":                                   Advisory,
		"trivy fs --exit-code 0 --format sarif -o t.sarif .\ntrivy fs --exit-code 1 --severity HIGH .":                  Enforcing,
		"trivy fs --exit-code 0 .": Advisory,
	} {
		t.Run(run, func(t *testing.T) {
			verdicts := gradeFile(t, parser.NewGithubParser(), ".github/workflows/ci.yml", `
on: pull_request
jobs:
  scan:
    steps:
      - run: |
          `+strings.ReplaceAll(run, "\n", "\n          ")+`
`)
			assert.Equal(t, expected, verdicts["SCA"])
		})
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

func TestStyLuaExitsZeroUnlessCheck(t *testing.T) {
	for run, expected := range map[string]Verdict{
		"stylua --check .":                        Enforcing,
		"stylua -c src/":                          Enforcing,
		"npx @johnnymorganz/stylua-bin --check .": Enforcing,
		"stylua .":                                Advisory,
		"stylua --verify lua/":                    Advisory,
		// The pre-commit wrapper marks a hook command; pre-commit fails a hook that changes a file.
		"ZANADIR_PRECOMMIT_HOOK=1 stylua .":        Enforcing,
		"CI=1 ZANADIR_PRECOMMIT_HOOK=1 stylua .":   Enforcing,
		"stylua --glob ZANADIR_PRECOMMIT_HOOK=1 .": Advisory,
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

func TestStyLuaActionArgs(t *testing.T) {
	tests := []struct {
		name     string
		options  string
		expected Verdict
	}{
		{"check", "        with:\n          args: --check .\n", Enforcing},
		{"check after other flags", "        with:\n          args: --color always --check .\n", Enforcing},
		{"formats only", "        with:\n          args: .\n", Advisory},
		{"install only", "        with:\n          args: false\n", Advisory},
		{"no args", "", Advisory},
		{"expression", "        with:\n          args: ${{ inputs.args }}\n", Unknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			verdicts := gradeFile(t, parser.NewGithubParser(), ".github/workflows/ci.yml", `
on: pull_request
jobs:
  lint:
    steps:
      - uses: JohnnyMorganz/stylua-action@v5
`+tt.options)
			assert.Equal(t, tt.expected, verdicts["Linter"])
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
		"npx cspell lint .":                Enforcing,
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

// Size Limit fails on an exceeded limit unless --why, which exits 0.
func TestSizeLimitWhyGradesAdvisory(t *testing.T) {
	for run, expected := range map[string]Verdict{
		"npx size-limit":       Enforcing,
		"npx size-limit --why": Advisory,
	} {
		verdicts := gradeFile(t, parser.NewGithubParser(), ".github/workflows/ci.yml", `
on: pull_request
jobs:
  size:
    steps:
      - run: `+run+`
`)
		assert.Equal(t, expected, verdicts["Performance Testing"], run)
	}
}

// Both size-limit actions fail the job when a limit is exceeded.
func TestSizeLimitActionsGradeEnforcing(t *testing.T) {
	for _, uses := range []string{"andresz1/size-limit-action@v1", "ant-design/size-limit-action@master"} {
		verdicts := gradeFile(t, parser.NewGithubParser(), ".github/workflows/ci.yml", `
on: pull_request
jobs:
  size:
    steps:
      - uses: `+uses+`
        with:
          github_token: ${{ secrets.GITHUB_TOKEN }}
`)
		assert.Equal(t, Enforcing, verdicts["Performance Testing"], uses)
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
		{"knip", "Linter", step("run: npx knip"), Enforcing},
		{"knip zero budget", "Linter", step("run: knip --max-issues 0"), Enforcing},
		{"knip no exit code", "Linter", step("run: pnpm knip --no-exit-code"), Advisory},
		{"knip budget", "Linter", step("run: knip --max-issues 50"), Partial},
		{"knip budget equals", "Linter", step("run: knip --max-issues=50"), Partial},
		{"knip fix", "Linter", step("run: pnpm knip --fix"), Partial},
		{"knip fix type", "Linter", step("run: knip --fix-type exports,types"), Partial},
		{"knip format flag", "Linter", step("run: knip -F"), Enforcing},
		{"knip reporter", "Linter", step("run: knip --reporter json > knip.json"), Enforcing},
		{"pre-commit staged files", "Linter", step("run: pre-commit run --show-diff-on-failure"), Advisory},
		{"pre-commit one hook", "Linter", step("uses: pre-commit/action@v3.0.1", "with:", "  extra_args: trailing-whitespace"), Advisory},
		{"pre-commit arguments", "Linter", step("run: pre-commit run $PRE_COMMIT_ARGS"), Unknown},
		{"pre-commit all files", "Linter", step("run: uv run pre-commit run -a"), Enforcing},
		{"pre-commit refs", "Linter", step("run: pre-commit run --from-ref origin/main --to-ref HEAD"), Enforcing},
		{"pre-commit abbreviated all files", "Linter", step("run: pre-commit run --all --show-diff-on-failure"), Enforcing},
		{"pre-commit shortest abbreviation", "Linter", step("run: pre-commit run --a"), Enforcing},
		{"pre-commit abbreviated in quotes", "Linter", step(`run: bash -c "pre-commit run --all"`), Enforcing},
		{"pre-commit abbreviated refs", "Linter", step("run: pre-commit run --fr origin/main --to HEAD"), Enforcing},
		{"pre-commit short flags together", "Linter", step("run: pre-commit run -av generate-actions-workflow"), Enforcing},
		{"pre-commit verbose then all", "Linter", step("run: pre-commit run -va"), Enforcing},
		{"pre-commit show diff abbreviated", "Linter", step("run: pre-commit run --show-diff -v"), Advisory},
		{"pre-commit config is not a selection", "Linter", step("run: pre-commit run -c .pre-commit-ci.yaml"), Advisory},
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

// basedpyright and pyrefly check fail on errors with no flag to turn it off.
func TestBasedpyrightAndPyreflyGradesEnforcing(t *testing.T) {
	for run, expected := range map[string]Verdict{
		"uv run --no-sync basedpyright":        Enforcing,
		"basedpyright --level error":           Enforcing,
		"pyrefly check --output-format=github": Enforcing,
		"uv run pyrefly check --summary=none":  Enforcing,
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

func TestPyreflyActionGradesCorrectly(t *testing.T) {
	for _, tc := range []struct {
		name            string
		args, stepExtra string
		expected        Verdict
	}{
		{name: "no input is enforcing", expected: Enforcing},
		{name: "args --summarize-errors is still enforcing", args: "--summarize-errors", expected: Enforcing},
		{name: "continue-on-error is advisory", stepExtra: "continue-on-error: true", expected: Advisory},
	} {
		t.Run(tc.name, func(t *testing.T) {
			workflow := `on: pull_request
jobs:
  typecheck:
    steps:
      - uses: facebook/pyrefly@main
`
			if tc.stepExtra != "" {
				workflow += "        " + tc.stepExtra + "\n"
			}
			if tc.args != "" {
				workflow += "        with:\n          args: " + tc.args + "\n"
			}
			verdicts := gradeFile(t, parser.NewGithubParser(), ".github/workflows/ci.yml", workflow)
			assert.Equal(t, tc.expected, verdicts["Linter"])
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

// Trivy and grype read their settings from the environment, which a
// workflow sets as often in env: as before the command.
func TestEnvSettingsAreRead(t *testing.T) {
	for _, tc := range []struct {
		name, workflowEnv, jobEnv, stepEnv, run, category string
		expected                                          Verdict
	}{
		{name: "step env fails", stepEnv: "TRIVY_EXIT_CODE: 1", run: "trivy fs .", category: "SCA", expected: Enforcing},
		{name: "job env fails", jobEnv: "TRIVY_EXIT_CODE: '1'", run: "trivy fs .", category: "SCA", expected: Enforcing},
		{name: "workflow env fails", workflowEnv: "TRIVY_EXIT_CODE: 1", run: "trivy fs .", category: "SCA", expected: Enforcing},
		{name: "step env outranks job env", jobEnv: "TRIVY_EXIT_CODE: 1", stepEnv: "TRIVY_EXIT_CODE: 0", run: "trivy fs .", category: "SCA", expected: Advisory},
		{name: "env zero", stepEnv: "TRIVY_EXIT_CODE: 0", run: "trivy fs .", category: "SCA", expected: Advisory},
		{name: "env expression", stepEnv: "TRIVY_EXIT_CODE: ${{ inputs.exit-code }}", run: "trivy fs .", category: "SCA", expected: Unknown},
		{name: "env config", stepEnv: "TRIVY_CONFIG: ci/trivy.yaml", run: "trivy fs .", category: "SCA", expected: Unknown},
		{name: "grype severity", stepEnv: "GRYPE_FAIL_ON_SEVERITY: high", run: "grype dir:.", category: "SCA", expected: Enforcing},
		{name: "unread variables", stepEnv: "GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}\n          CONFIG: ${{ inputs.config }}", run: "trivy fs .", category: "SCA", expected: Advisory},
		{name: "unread dollar", jobEnv: "PATH_EXTRA: $HOME/bin", run: "pre-commit run --all-files", category: "Linter", expected: Enforcing},
		{name: "env set by expression", jobEnv: "${{ fromJSON(inputs.env) }}", run: "trivy fs --exit-code 1 .", category: "SCA", expected: Enforcing},
	} {
		t.Run(tc.name, func(t *testing.T) {
			block := func(indent, env string) string {
				if env == "" {
					return ""
				}
				if strings.HasPrefix(env, "${{") {
					return indent + "env: " + env + "\n"
				}
				return indent + "env:\n" + indent + "  " + env + "\n"
			}
			verdicts := gradeFile(t, parser.NewGithubParser(), ".github/workflows/ci.yml", "on: pull_request\n"+
				block("", tc.workflowEnv)+"jobs:\n  scan:\n"+block("    ", tc.jobEnv)+
				"    steps:\n      - run: "+tc.run+"\n"+block("        ", tc.stepEnv))
			assert.Equal(t, tc.expected, verdicts[tc.category])
		})
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

// typos exits 2 when it finds a typo; -w / --write-changes fixes every typo
// that has a single correction and only fails on the rest, so it is a fixer,
// not a check.
func TestTyposWriteChangesGradesAdvisory(t *testing.T) {
	for run, expected := range map[string]Verdict{
		"typos":                               Enforcing,
		"typos --config=./.config/typos.toml": Enforcing,
		"typos -w":                            Advisory,
		"typos --write-changes":               Advisory,
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

// The crate-ci/typos action fails on typos unless write_changes fixes them
// away; a non-literal write_changes input cannot be graded.
func TestTyposActionWriteChangesInputGradesCorrectly(t *testing.T) {
	for _, tc := range []struct {
		name     string
		input    string
		expected Verdict
	}{
		{name: "default is enforcing", expected: Enforcing},
		{name: "write_changes true is advisory", input: "write_changes: true", expected: Advisory},
		{name: "expression is unknown", input: "write_changes: ${{ inputs.fix }}", expected: Unknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			workflow := `on: pull_request
jobs:
  spelling:
    steps:
      - uses: crate-ci/typos@v1
`
			if tc.input != "" {
				workflow += "        with:\n          " + tc.input + "\n"
			}
			verdicts := gradeFile(t, parser.NewGithubParser(), ".github/workflows/ci.yml", workflow)
			assert.Equal(t, tc.expected, verdicts["Linter"])
		})
	}
}

func TestTerraformDocsExitsZeroUnlessOutputCheck(t *testing.T) {
	for run, expected := range map[string]Verdict{
		"terraform-docs --output-check .":                             Enforcing,
		"terraform-docs -c .terraform-docs.yml --output-check=true .": Enforcing,
		"terraform-docs markdown table --output-file README.md .":     Advisory,
		"terraform-docs --output-check=false .":                       Advisory,
	} {
		verdicts := gradeFile(t, parser.NewGithubParser(), ".github/workflows/ci.yml", `
on: pull_request
jobs:
  docs:
    steps:
      - run: `+run+`
`)
		assert.Equal(t, expected, verdicts["Linter"], run)
	}
}

func TestTaploCheckGradesEnforcing(t *testing.T) {
	for _, run := range []string{
		"taplo fmt --check --diff",
		`taplo lint --schema "file://${GITHUB_WORKSPACE}/schema.json" presets/*.toml`,
		"npx --yes @taplo/cli@0.7.0 fmt --check",
	} {
		verdicts := gradeFile(t, parser.NewGithubParser(), ".github/workflows/ci.yml", `
on: pull_request
jobs:
  toml:
    steps:
      - run: `+run+`
`)
		assert.Equal(t, Enforcing, verdicts["Linter"], run)
	}
}

func TestTaploFmtWithoutCheckIsNotALinter(t *testing.T) {
	for _, run := range []string{"taplo fmt", "taplo fmt --diff"} {
		verdicts := gradeFile(t, parser.NewGithubParser(), ".github/workflows/ci.yml", `
on: pull_request
jobs:
  toml:
    steps:
      - run: `+run+`
`)
		assert.NotContains(t, verdicts, "Linter", run)
	}
}

func TestTerraformDocsActionFailOnDiff(t *testing.T) {
	for _, tc := range []struct {
		name     string
		inputs   string
		expected Verdict
	}{
		{name: "no inputs is advisory", expected: Advisory},
		{name: "fail-on-diff true is enforcing", inputs: "          fail-on-diff: true\n", expected: Enforcing},
		{name: "fail-on-diff true with git-push true is advisory", inputs: "          fail-on-diff: true\n          git-push: true\n", expected: Advisory},
		{name: "expression input is unknown", inputs: "          fail-on-diff: ${{ inputs.check }}\n", expected: Unknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			workflow := "on: pull_request\njobs:\n  docs:\n    steps:\n      - uses: terraform-docs/gh-actions@v1.4.1\n"
			if tc.inputs != "" {
				workflow += "        with:\n" + tc.inputs
			}
			verdicts := gradeFile(t, parser.NewGithubParser(), ".github/workflows/ci.yml", workflow)
			assert.Equal(t, tc.expected, verdicts["Linter"])
		})
	}
}

// check-jsonschema validates JSON, YAML, and TOML schemas and exits 1 on failure.
func TestCheckJsonschemaGradesCorrectly(t *testing.T) {
	for run, expected := range map[string]Verdict{
		"check-jsonschema --builtin-schema vendor.dependabot .github/dependabot.yml": Enforcing,
		"uvx check-jsonschema --schemafile schema.json data.json":                    Enforcing,
		"check-jsonschema --disable-formats '*' --schemafile schema.json data.json":  Enforcing,
		"check-github-workflows": Enforcing,
		"check-dependabot":       Enforcing,
		"check-jsonschema --builtin-schema vendor.dependabot .github/dependabot.yml || true": Advisory,
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

func TestGcovrGradesByTriggers(t *testing.T) {
	for run, expected := range map[string]Verdict{
		"gcovr --root . --xml-pretty -o coverage.xml":         Enforcing,
		"gcovr -r . --fail-under-line 80":                     Enforcing,
		"uv run --locked gcovr --root . --txt":                Enforcing,
		"gcovr --root . --xml-pretty -o coverage.xml || true": Advisory,
	} {
		verdicts := gradeFile(t, parser.NewGithubParser(), ".github/workflows/ci.yml", `
on: pull_request
jobs:
  coverage:
    steps:
      - run: `+run+`
`)
		assert.Equal(t, expected, verdicts["Coverage"], run)
	}
}

func TestGcovrActionGradesEnforcing(t *testing.T) {
	for _, with := range []string{
		"",
		"\n        with:\n          fail-under-line: 80",
	} {
		t.Run(with, func(t *testing.T) {
			verdicts := gradeFile(t, parser.NewGithubParser(), ".github/workflows/ci.yml", `
on: pull_request
jobs:
  coverage:
    steps:
      - uses: threeal/gcovr-action@v1.2.0`+with+`
`)
			assert.Equal(t, Enforcing, verdicts["Coverage"])
		})
	}
}

// oasdiff exits 0 on breaking changes unless --fail-on or -o is given.
func TestOasdiffExitsZeroUnlessFailOn(t *testing.T) {
	for run, expected := range map[string]Verdict{
		"oasdiff breaking base.yaml revision.yaml":                                    Advisory,
		"oasdiff breaking --fail-on ERR base.yaml revision.yaml":                      Enforcing,
		"oasdiff breaking -o WARN base.yaml revision.yaml":                            Enforcing,
		"oasdiff breaking-files openapi.yaml --base origin/main --fail-on ERR":        Enforcing,
		"oasdiff --config ci/oasdiff.yaml breaking base.yaml revision.yaml":           Unknown,
	} {
		verdicts := gradeFile(t, parser.NewGithubParser(), ".github/workflows/ci.yml", `
on: pull_request
jobs:
  api:
    steps:
      - run: `+run+`
`)
		assert.Equal(t, expected, verdicts["Linter"], run)
	}
}

// oasdiff/oasdiff-action/breaking exits 0 unless fail-on is set.
func TestOasdiffActionFailOnGradesAdvisory(t *testing.T) {
	for _, tc := range []struct {
		name     string
		uses     string
		input    string
		expected Verdict
	}{
		{
			name:     "fail-on ERR is enforcing",
			uses:     "oasdiff/oasdiff-action/breaking@v0",
			input:    "fail-on: ERR",
			expected: Enforcing,
		},
		{
			name:     "fail-on warn is enforcing",
			uses:     "oasdiff/oasdiff-action/breaking@v0",
			input:    "fail-on: warn",
			expected: Enforcing,
		},
		{
			name:     "without fail-on is advisory",
			uses:     "oasdiff/oasdiff-action/breaking@v0",
			expected: Advisory,
		},
		{
			name:     "dynamic fail-on is unknown",
			uses:     "oasdiff/oasdiff-action/breaking@v0",
			input:    "fail-on: ${{ inputs.fail-on }}",
			expected: Unknown,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			workflow := `on: pull_request
jobs:
  api:
    steps:
      - uses: ` + tc.uses + `
        with:
          base: base.yaml
          revision: revision.yaml
`
			if tc.input != "" {
				workflow += "          " + tc.input + "\n"
			}
			verdicts := gradeFile(t, parser.NewGithubParser(), ".github/workflows/ci.yml", workflow)
			assert.Equal(t, tc.expected, verdicts["Linter"])
		})
	}
}

