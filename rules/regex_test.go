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
			ruleID:         "codecov-rule",
			shouldMatch:    []string{"codecov/codecov-action", "bash <(curl -s https://codecov.io/bash)"},
			shouldNotMatch: []string{"mycodecovx"},
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
