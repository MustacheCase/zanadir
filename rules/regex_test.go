package rules_test

import (
	"slices"
	"strings"
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
			ruleID: "phpcs-rule",
			shouldMatch: []string{
				"phpcs --standard=PSR12 src/",
				"vendor/bin/phpcs -p",
				"composer exec phpcs -- --report=checkstyle",
				"./vendor/bin/phpcs",
				"npx phpcs",
			},
			shouldNotMatch: []string{
				"phpcbf src/",
				"cp phpcs.xml.dist phpcs.xml",
				"php-cs-fixer fix --dry-run",
				"phpcs.xml",
				"phpcs.xml.dist",
			},
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
