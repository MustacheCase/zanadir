package rules_test

import (
	"testing"

	"github.com/MustacheCase/zanadir/models"
	"github.com/MustacheCase/zanadir/rules"
	"github.com/stretchr/testify/assert"
)

// findRule looks a rule up by id across every category.
func findRule(t *testing.T, rs rules.RuleService, id string) *rules.Rule {
	t.Helper()
	for _, category := range models.CategoryTitles {
		for _, r := range rs.GetCategoryRules(category) {
			if r.ID == id {
				return r
			}
		}
	}
	t.Fatalf("rule %q not found", id)
	return nil
}

// The regexes were bare substrings. The shouldNotMatch cases are lookalikes
// that used to produce a false "this category is covered".
func TestRuleRegexPrecision(t *testing.T) {
	rs, err := rules.NewRulesService()
	assert.NoError(t, err)

	tests := []struct {
		ruleID         string
		shouldMatch    []string
		shouldNotMatch []string
	}{
		{
			ruleID:         "trivy-rule",
			shouldMatch:    []string{"trivy fs .", "trivy image --exit-code 1 alpine", "trivy -q repo .", "trivy --cache-dir /tmp/t rootfs /", "trivy sbom sbom.json", "docker run aquasec/trivy filesystem /src"},
			shouldNotMatch: []string{"trivyscanner", "mytrivy fs", "trivy config .", "trivy --version", "trivy image-something", "aquasecurity/trivy-action"},
		},
		{
			ruleID:         "sonar-rule",
			shouldMatch:    []string{"SonarCloudAnalyze", "SonarQubeAnalyze", "sonarsource/sonarcloud-github-action", "sonar-scanner", "withSonarQubeEnv", "waitForQualityGate"},
			shouldNotMatch: []string{"SonarCloudPrepare", "SonarQubePublish", "sonarlint"},
		},
		{
			ruleID:         "snyk-rule",
			shouldMatch:    []string{"SnykSecurityScan", "snyk test", "snyk/snyk-scan", "snykSecurity"},
			shouldNotMatch: []string{"snykish", "SnykSecurityScanner"},
		},
		{
			ruleID:         "azure-dependency-scanning-rule",
			shouldMatch:    []string{"AdvancedSecurity-Dependency-Scanning"},
			shouldNotMatch: []string{"AdvancedSecurity-Codeql-Analyze", "AdvancedSecurity-Dependency-Scanning-Extra"},
		},
		{
			ruleID:         "coverage-results-step-rule",
			shouldMatch:    []string{"PublishCodeCoverageResults", "recordCoverage", "publishCoverage"},
			shouldNotMatch: []string{"PublishTestResults", "PublishCodeCoverageResultsV2Wrapper"},
		},
		{
			ruleID:         "test-results-step-rule",
			shouldMatch:    []string{"VSTest", "PublishTestResults", "junit"},
			shouldNotMatch: []string{"VSBuild", "PublishTestResultsLegacy"},
		},
		{
			ruleID:         "gitlab-sast-template-rule",
			shouldMatch:    []string{"Jobs/SAST.gitlab-ci.yml", "Security/SAST.gitlab-ci.yml", "Jobs/SAST.latest.gitlab-ci.yml", "$CI_SERVER_FQDN/components/sast/sast@2.0.2"},
			shouldNotMatch: []string{"Jobs/SAST-IaC.gitlab-ci.yml", "Security/DAST.gitlab-ci.yml", "my/SAST.gitlab-ci.yml"},
		},
		{
			ruleID:         "gitlab-sast-iac-template-rule",
			shouldMatch:    []string{"Jobs/SAST-IaC.gitlab-ci.yml", "Security/SAST-IaC.latest.gitlab-ci.yml"},
			shouldNotMatch: []string{"Jobs/SAST.gitlab-ci.yml"},
		},
		{
			ruleID:         "gitlab-secret-detection-template-rule",
			shouldMatch:    []string{"Jobs/Secret-Detection.gitlab-ci.yml", "Security/Secret-Detection.gitlab-ci.yml", "gitlab.com/components/secret-detection/secret-detection@1"},
			shouldNotMatch: []string{"Jobs/SAST.gitlab-ci.yml"},
		},
		{
			ruleID:         "gitlab-dependency-scanning-template-rule",
			shouldMatch:    []string{"Jobs/Dependency-Scanning.latest.gitlab-ci.yml", "Security/Dependency-Scanning.gitlab-ci.yml", "Jobs/Dependency-Scanning.v2.gitlab-ci.yml", "Security/Container-Scanning.gitlab-ci.yml", "$CI_SERVER_FQDN/components/container-scanning/container-scanning@6"},
			shouldNotMatch: []string{"Jobs/Code-Quality.gitlab-ci.yml", "Security/DAST.gitlab-ci.yml"},
		},
		{
			ruleID:         "gitlab-license-scanning-template-rule",
			shouldMatch:    []string{"Security/License-Scanning.gitlab-ci.yml"},
			shouldNotMatch: []string{"Security/Secret-Detection.gitlab-ci.yml"},
		},
		{
			ruleID:         "gitlab-code-quality-template-rule",
			shouldMatch:    []string{"Jobs/Code-Quality.gitlab-ci.yml", "Code-Quality.gitlab-ci.yml"},
			shouldNotMatch: []string{"Jobs/Code-Intelligence.gitlab-ci.yml"},
		},
		{
			ruleID:         "gitlab-performance-template-rule",
			shouldMatch:    []string{"Jobs/Browser-Performance-Testing.gitlab-ci.yml", "Verify/Load-Performance-Testing.gitlab-ci.yml", "Verify/Browser-Performance.gitlab-ci.yml"},
			shouldNotMatch: []string{"Security/DAST.gitlab-ci.yml"},
		},
		{
			ruleID:         "dependency-check-rule",
			shouldMatch:    []string{"dependencyCheck", "dependencyCheckPublisher", "dependency-check.sh --scan .", "dependency-check --project x"},
			shouldNotMatch: []string{"dependencyCheckBomName", "my-dependency-checker"},
		},
		{
			ruleID:         "jenkins-record-issues-rule",
			shouldMatch:    []string{"recordIssues"},
			shouldNotMatch: []string{"recordIssuesLater"},
		},
		{
			ruleID:         "git-secrets-rule",
			shouldMatch:    []string{"atlassian/git-secrets-scan", "git secrets --scan", "git-secrets --scan-history"},
			shouldNotMatch: []string{"git-secretsmanager", "github secrets", "git secret reveal"},
		},
		{
			ruleID:         "trivy-eol-rule",
			shouldMatch:    []string{"trivy image --exit-on-eol 1 zanadir:eol-scan", "exit-on-eol: 1"},
			shouldNotMatch: []string{"trivy fs .", "aquasecurity/trivy-action", "trivy sbom"},
		},
		{
			// Scout is used for CVEs far more often than for base-image age, so
			// the bare command must not count as end-of-life coverage.
			ruleID:         "docker-scout-eol-rule",
			shouldMatch:    []string{"docker scout policy --only-eol", "scout recommendations --eol"},
			shouldNotMatch: []string{"docker scout cves", "docker/scout-action", "scouting the eol", "eol scout"},
		},
		{
			// The bots are reported by the dependency-bot parser with the tool
			// name as the whole package. dependabot/fetch-metadata is a real
			// action for reading pull request metadata, and repositories have
			// entire workflows named dependabot-triage; neither is dependency
			// scanning, so the regex is anchored.
			ruleID:         "dependabot-rule",
			shouldMatch:    []string{"dependabot"},
			shouldNotMatch: []string{"dependabot/fetch-metadata", "dependabot-triage", "my-dependabot", "dependabot-core"},
		},
		{
			ruleID:         "renovate-rule",
			shouldMatch:    []string{"renovate"},
			shouldNotMatch: []string{"renovatebot/github-action", "renovate-config", "auto-renovate"},
		},
		{
			ruleID:         "bouncer-rule",
			shouldMatch:    []string{"bouncer check ./...", ".tool/bouncer list", "github.com/wagoodman/go-bouncer"},
			shouldNotMatch: []string{"bouncer", "deploy-bouncer-service", "bouncercheck"},
		},
		{
			ruleID:         "ruff-rule",
			shouldMatch:    []string{"uv run ruff check .", "astral-sh/ruff-action", "ruff format --check"},
			shouldNotMatch: []string{"ruffle", "gruff"},
		},
		{
			ruleID:         "python-type-checker-rule",
			shouldMatch:    []string{"uv run mypy --config-file ./pyproject.toml src", "pyright", "jakebailey/pyright-action"},
			shouldNotMatch: []string{"mypyc-build", "pyrightish"},
		},
		{
			ruleID: "coverage-command-rule",
			shouldMatch: []string{
				"uv run pytest --cov vunnel -v tests/unit/", "pytest tests --cov=src --cov-report=xml",
				"coverage run -m unittest", "go test ./... -coverprofile=coverage.txt", "go test -race -cover ./...",
				"npx jest --coverage", "vitest run --coverage", "nyc mocha", "cargo llvm-cov --lcov",
				"cargo tarpaulin --out Xml", "vendor/bin/phpunit --coverage-clover clover.xml",
				`dotnet test --collect:"XPlat Code Coverage"`, "vladopajic/go-test-coverage",
			},
			shouldNotMatch: []string{
				"pytest tests/", "go test ./...", "go test -run TestCoverage ./...", "jest", "npm run coverage-badge",
				"pytest --covid", "go test -coverage-of-nothing",
			},
		},
		{
			ruleID: "grype-rule",
			shouldMatch: []string{"grype dir:.", "anchore/grype", "./bin/grype sbom:sbom.json --fail-on high",
				"docker run anchore/grype:latest alpine", "set -e\ngrype .", "anchore/grype@v0.80.0",
				"$CI_SERVER_HOST/proton/devops/cicd-components/devsecops/grype/scan-code@~latest"},
			shouldNotMatch: []string{"grypeless", "go run cmd/syft/main.go anchore/test_images:grype-quality-dotnet-69f15d2",
				"grype-db build", "cat .grype.yaml"},
		},
		{
			ruleID:         "zizmor-rule",
			shouldMatch:    []string{"zizmorcore/zizmor-action", "zizmor .github/workflows", "pipx run zizmor"},
			shouldNotMatch: []string{"zizmorish", "myzizmor"},
		},
		{
			ruleID:         "actionlint-rule",
			shouldMatch:    []string{"rhysd/actionlint", "reviewdog/action-actionlint", "actionlint -color"},
			shouldNotMatch: []string{"actionlinter", "action lint"},
		},
		{
			// The plural is a different thing, and the boundary excludes it.
			ruleID:         "scorecard-rule",
			shouldMatch:    []string{"ossf/scorecard-action", "Scorecard analysis", "scorecard --repo=."},
			shouldNotMatch: []string{"scorecards", "scorecarding"},
		},
		{
			ruleID:         "govulncheck-rule",
			shouldMatch:    []string{"govulncheck ./...", "golang/govulncheck-action", "go run golang.org/x/vuln/cmd/govulncheck@latest ./..."},
			shouldNotMatch: []string{"govulncheckers", "mygovulncheck"},
		},
		{
			ruleID:         "osv-scanner-rule",
			shouldMatch:    []string{"osv-scanner -r .", "google/osv-scanner-action"},
			shouldNotMatch: []string{"osv-scannerx", "osv database"},
		},
		{
			ruleID:         "pip-audit-rule",
			shouldMatch:    []string{"pip-audit -r requirements.txt", "pypa/gh-action-pip-audit"},
			shouldNotMatch: []string{"pip-auditor", "pip install audit"},
		},
		{
			// "audit" on its own is in half the workflow names on GitHub, so
			// only the subcommand counts.
			ruleID:         "npm-audit-rule",
			shouldMatch:    []string{"npm audit", "npm audit --audit-level=high", "yarn audit", "pnpm audit --prod"},
			shouldNotMatch: []string{"npm auditor", "audit", "security audit job", "npm install", "npm run audit-ci"},
		},
		{
			ruleID:         "snyk-rule",
			shouldMatch:    []string{"snyk test", "snyk/actions/golang"},
			shouldNotMatch: []string{"snyking"},
		},
		{
			ruleID:         "gitleaks-rule",
			shouldMatch:    []string{"gitleaks detect --source .", "gitleaks/gitleaks-action"},
			shouldNotMatch: []string{"gitleaksy"},
		},
		{
			ruleID:         "trufflehog-rule",
			shouldMatch:    []string{"trufflehog git file://.", "trufflesecurity/trufflehog"},
			shouldNotMatch: []string{"trufflehogsx"},
		},
		{
			ruleID:         "detect-secrets-rule",
			shouldMatch:    []string{"detect-secrets scan", "detect_secrets scan"},
			shouldNotMatch: []string{"detect-secretsfoo"},
		},
		{
			// "grant" is an English word and a SQL keyword.
			ruleID:         "grant-rule",
			shouldMatch:    []string{"grant check", "anchore/grant", "grant list"},
			shouldNotMatch: []string{`psql -c "GRANT ALL PRIVILEGES ON db TO user"`, "grants-management", "grant the token access"},
		},
		{
			ruleID:         "golangci-lint-rule",
			shouldMatch:    []string{"golangci-lint run ./...", "golangci/golangci-lint-action"},
			shouldNotMatch: []string{"mygolangci-lintx"},
		},
		{
			ruleID:         "eslint-rule",
			shouldMatch:    []string{"eslint .", "npx eslint --fix"},
			shouldNotMatch: []string{"eslintrc", "myeslint"},
		},
		{
			// "latest" contains "test", which ".*test.*" matched.
			ruleID:         "unit-test-rule",
			shouldMatch:    []string{"test", "unit-tests", "run-tests", "go test ./...", "build-and-test", "go-test-race", "release-test"},
			shouldNotMatch: []string{"latest", "deploy-latest", "protest-build", "contest"},
		},
		{
			// A bare "ab" matched any standalone occurrence.
			ruleID:         "apache-bench-rule",
			shouldMatch:    []string{"apache-bench -n 100", "apachebench", "ab -n 1000 http://x/"},
			shouldNotMatch: []string{"ab testing", "build ab", "grab a coffee"},
		},
		{
			ruleID:         "semgrep-rule",
			shouldMatch:    []string{"semgrep ci", "semgrep/semgrep-action", "semgrep scan --config auto"},
			shouldNotMatch: []string{"semgreppy", "mysemgrep"},
		},
		{
			ruleID: "codeql-rule",
			shouldMatch: []string{"github/codeql-action/analyze@v3", "github/codeql-action/init", "github/codeql-action/autobuild",
				"codeql database analyze db --format=sarif-latest", "AdvancedSecurity-Codeql-Init@1"},
			shouldNotMatch: []string{"codeqlish", "github/codeql-action/upload-sarif@v3", "github/codeql-action/upload-sarif",
				"echo codeql"},
		},
		{
			ruleID:         "codeql-name-rule",
			shouldMatch:    []string{"CodeQL", "CodeQL Advanced", "Run CodeQL Analysis"},
			shouldNotMatch: []string{"codeqlish"},
		},
		{
			// A bare "sonar" appears in unrelated project and job names.
			ruleID:         "sonar-rule",
			shouldMatch:    []string{"SonarSource/sonarqube-scan-action", "sonar-scanner -Dsonar.host.url=x", "sonarcloud scan"},
			shouldNotMatch: []string{"sonar", "sonarian", "build-sonar-module"},
		},
		{
			ruleID:         "gosec-rule",
			shouldMatch:    []string{"gosec ./...", "securego/gosec@master"},
			shouldNotMatch: []string{"gosecure", "mygosecx"},
		},
		{
			ruleID:         "bandit-rule",
			shouldMatch:    []string{"bandit -r .", "PyCQA/bandit"},
			shouldNotMatch: []string{"banditry", "mybandits"},
		},
		{
			// "snyk" alone is the SCA rule; SAST is specifically "snyk code".
			ruleID:         "snyk-code-rule",
			shouldMatch:    []string{"snyk code test", "snyk  code  test --json"},
			shouldNotMatch: []string{"snyk test", "snyk monitor", "snyk container test"},
		},
		{
			// "Bearer" is ubiquitous in auth headers and must never match alone.
			ruleID:         "bearer-rule",
			shouldMatch:    []string{"bearer/bearer-action@v2", "bearer scan ."},
			shouldNotMatch: []string{"Authorization: Bearer ${{ secrets.GITHUB_TOKEN }}", "bearer token", "-H \"Authorization: Bearer abc\""},
		},
		{
			ruleID:         "checkov-rule",
			shouldMatch:    []string{"checkov -d .", "bridgecrewio/checkov-action@master"},
			shouldNotMatch: []string{"checkovx", "mycheckov"},
		},
		{
			ruleID:         "tfsec-rule",
			shouldMatch:    []string{"tfsec .", "aquasecurity/tfsec-action"},
			shouldNotMatch: []string{"tfsecure", "mytfsec"},
		},
		{
			ruleID:         "kics-rule",
			shouldMatch:    []string{"kics scan -p .", "checkmarx/kics-github-action"},
			shouldNotMatch: []string{"kicks", "nkics"},
		},
		{
			// "snyk" alone is SCA; only the iac sub-command scans infrastructure.
			ruleID:         "snyk-iac-rule",
			shouldMatch:    []string{"snyk iac test", "snyk  iac  test ."},
			shouldNotMatch: []string{"snyk test", "snyk code test"},
		},
		{
			// "trivy" alone is SCA; the config sub-command is the misconfig scanner.
			ruleID:         "trivy-config-rule",
			shouldMatch:    []string{"trivy config .", "trivy  config  ./infra", "trivy -q conf .", "trivy fs --scanners vuln,misconfig .", "trivy repo --security-checks config ."},
			shouldNotMatch: []string{"trivy fs .", "trivy image alpine", "trivy fs --scanners vuln ."},
		},
		{
			ruleID:         "codecov-rule",
			shouldMatch:    []string{"codecov/codecov-action", "bash <(curl -s https://codecov.io/bash)"},
			shouldNotMatch: []string{"mycodecovx"},
		},
		{
			// A bare \btests?\b would count `if test -f` as coverage.
			ruleID: "unit-test-command-rule",
			shouldMatch: []string{
				"go test ./...", "go test -race -cover ./...", "npm test", "npm run test",
				"yarn test", "pnpm run test", "pytest -q", "cargo test --all",
				"dotnet test", "mvn -B verify test", "gradle clean test",
				"bundle exec rspec", "npx jest --ci", "mocha spec/", "vitest run", "tox -e py311",
				"./gradlew test", "./mvnw -B test", "gradle test -x integrationTest", "tox -e lint,py312", "tox",
				"mvn -Dmaven.test.skip=false test",
			},
			shouldNotMatch: []string{
				"if test -f go.mod; then echo yes; fi", `test -z "$VAR" && exit 1`,
				"curl https://test.example.com", `echo "testing the build"`,
				"go build ./...", "go vet ./...", "npm run build", "docker build -t app:test .",
				"latest", "deploy-latest",
			},
		},
		{
			ruleID:         "syft-rule",
			shouldMatch:    []string{"syft dir:. -o spdx-json", "anchore/sbom-action@v0"},
			shouldNotMatch: []string{"syftly", "mysyft"},
		},
		{
			ruleID:         "cosign-rule",
			shouldMatch:    []string{"cosign sign --yes $IMAGE"},
			shouldNotMatch: []string{"cosigning the release", "cosigner"},
		},
		{
			// A bare "spdx" matches the licence header, which says nothing
			// about whether an SBOM is produced.
			ruleID:      "spdx-tools-rule",
			shouldMatch: []string{"spdx-sbom-generator -p .", "spdx-tools convert"},
			shouldNotMatch: []string{
				"# SPDX-License-Identifier: MIT",
				"SPDX-License-Identifier: Apache-2.0",
				"spdx",
			},
		},
		{
			ruleID:         "slsa-rule",
			shouldMatch:    []string{"slsa-framework/slsa-github-generator", "slsa provenance"},
			shouldNotMatch: []string{"slsawesome", "salsa", "slsax"},
		},
		{
			ruleID:         "github-attestation-rule",
			shouldMatch:    []string{"actions/attest-build-provenance@v1", "actions/attest-sbom@v1"},
			shouldNotMatch: []string{"actions/checkout@v4", "attestation"},
		},
		{
			// trivy writes an SBOM as an output format; "trivy sbom" scans one.
			ruleID:         "trivy-sbom-rule",
			shouldMatch:    []string{"trivy fs --format cyclonedx --output sbom.json .", "trivy image -f spdx-json alpine"},
			shouldNotMatch: []string{"trivy fs .", "trivy image alpine", "trivy config .", "trivy sbom sbom.json", "trivy fs --format sarif ."},
		},
		{
			ruleID:         "trivy-secret-rule",
			shouldMatch:    []string{"trivy fs --scanners secret .", "trivy repo --scanners vuln,secret ."},
			shouldNotMatch: []string{"trivy fs .", "trivy fs --scanners vuln ."},
		},
	}

	for _, tt := range tests {
		t.Run(tt.ruleID, func(t *testing.T) {
			rule := findRule(t, rs, tt.ruleID)
			for _, s := range tt.shouldMatch {
				assert.True(t, rule.Regex.MatchString(s), "%s should match %q", tt.ruleID, s)
				if rule.Unless != nil {
					assert.False(t, rule.Unless.MatchString(s), "%s should not except %q", tt.ruleID, s)
				}
			}
			for _, s := range tt.shouldNotMatch {
				assert.False(t, rule.Regex.MatchString(s), "%s should NOT match %q", tt.ruleID, s)
			}
		})
	}
}

// A command a rule's regex matches, but that does not do what the rule credits.
func TestRuleUnlessExceptsLookalikeCommands(t *testing.T) {
	rs, err := rules.NewRulesService()
	assert.NoError(t, err)

	for id, excepted := range map[string][]string{
		"trivy-rule": {"trivy fs --scanners secret .", "trivy repo --scanners misconfig,secret .", "trivy fs --security-checks config .", "trivy image --format cyclonedx alpine", "trivy fs -f spdx ."},
		"unit-test-command-rule": {
			"mvn package -Dmaven.test.skip=true", "./mvnw -B install -DskipTests test", "gradle build -x test",
			"./gradlew check --exclude-task test", "cargo test --no-run", "cargo test --all --no-run --locked",
			"tox -e lint", "tox -e mypy,docs", "tox --envlist=pre-commit",
		},
		"unit-test-rule": {"deploy-test", "publish-test-pypi", "report-test-failure", "tests-result", "set-test-package-matrix", "Snyk Test", "Test GitHub OIDC"},
	} {
		rule := findRule(t, rs, id)
		for _, command := range excepted {
			assert.True(t, rule.Regex.MatchString(command), "%s should match %q before excepting it", id, command)
			assert.True(t, rule.Unless.MatchString(command), "%s should except %q", id, command)
		}
	}
}
