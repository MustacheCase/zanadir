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
			shouldMatch:    []string{"trivy fs .", "aquasecurity/trivy-action", "run trivy"},
			shouldNotMatch: []string{"trivyscanner", "mytrivy"},
		},
		{
			ruleID:         "grype-rule",
			shouldMatch:    []string{"grype dir:.", "anchore/grype"},
			shouldNotMatch: []string{"grypeless"},
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
			shouldMatch:    []string{"test", "unit-tests", "run-tests", "go test ./..."},
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
			ruleID:         "codeql-rule",
			shouldMatch:    []string{"github/codeql-action/analyze@v3", "codeql database analyze"},
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
			shouldMatch:    []string{"trivy config .", "trivy  config  ./infra"},
			shouldNotMatch: []string{"trivy fs .", "trivy image alpine"},
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
			shouldMatch:    []string{"cosign sign --yes $IMAGE", "sigstore/cosign-installer@v3"},
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
			// "trivy" alone is SCA; the sbom sub-command is supply chain.
			ruleID:         "trivy-sbom-rule",
			shouldMatch:    []string{"trivy sbom --format cyclonedx .", "trivy  sbom ."},
			shouldNotMatch: []string{"trivy fs .", "trivy image alpine", "trivy config ."},
		},
	}

	for _, tt := range tests {
		t.Run(tt.ruleID, func(t *testing.T) {
			rule := findRule(t, rs, tt.ruleID)
			for _, s := range tt.shouldMatch {
				assert.True(t, rule.Regex.MatchString(s), "%s should match %q", tt.ruleID, s)
			}
			for _, s := range tt.shouldNotMatch {
				assert.False(t, rule.Regex.MatchString(s), "%s should NOT match %q", tt.ruleID, s)
			}
		})
	}
}
