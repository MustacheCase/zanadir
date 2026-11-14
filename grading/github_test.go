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
