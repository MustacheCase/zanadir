package rules_test

import (
	"testing"

	"github.com/MustacheCase/zanadir/models"
	"github.com/MustacheCase/zanadir/rules"
	"github.com/stretchr/testify/assert"
)

func TestGetCategoryRules(t *testing.T) {
	// Initialize the service using the embedded rules.
	rs, err := rules.NewRulesService()
	if err != nil {
		t.Fatalf("failed to initialize rules service: %v", err)
	}

	// Change "testCategory" to a category that exists in your embedded JSON file.
	result := rs.GetCategoryRules(models.CategoryTitle("SCA"))
	if len(result) == 0 {
		t.Fatalf("expected at least one rule in category 'testCategory'")
	}
}

// Guards against a rule naming an applyOn selector or category that nothing
// handles, which leaves it silently matching nothing.
func TestEmbeddedRulesAreValid(t *testing.T) {
	_, err := rules.NewRulesService()
	assert.NoError(t, err)
}

// A weakenedWhen that is written wrongly would silently check nothing, which
// is the failure this project keeps getting bitten by.
func TestValidateWeakenedRejectsBadChecks(t *testing.T) {
	tests := []struct {
		name     string
		check    rules.FileWeakenedWhen
		contains string
	}{
		{
			name:     "neither runMatches nor input",
			check:    rules.FileWeakenedWhen{Verdict: "advisory"},
			contains: "needs runMatches or input",
		},
		{
			name:     "both runMatches and input",
			check:    rules.FileWeakenedWhen{RunMatches: "x", Input: "y", Equals: "z", Verdict: "advisory"},
			contains: "not both",
		},
		{
			name:     "an input with nothing to compare against",
			check:    rules.FileWeakenedWhen{Input: "soft_fail", Verdict: "advisory"},
			contains: "needs equals",
		},
		{
			name:     "a verdict grading would not report",
			check:    rules.FileWeakenedWhen{RunMatches: "x", Verdict: "enforcing"},
			contains: "expected one of",
		},
		{
			name:     "no verdict at all",
			check:    rules.FileWeakenedWhen{RunMatches: "x"},
			contains: "expected one of",
		},
		{
			name:     "a regex that does not compile",
			check:    rules.FileWeakenedWhen{RunMatches: "[unclosed", Verdict: "advisory"},
			contains: "error parsing regexp",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := rules.ValidateRulesForTest([]rules.FileRule{{
				ID:         "x-rule",
				ApplyOn:    []string{rules.FieldJobRun},
				Categories: []string{"SCA"},
				Regex:      "x",
				Weakened:   []rules.FileWeakenedWhen{tt.check},
			}})
			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.contains)
		})
	}
}

// The shipped rules must actually carry their checks through to compiled form,
// or the feature is dead data in a YAML file.
func TestEmbeddedRulesCarryWeakenedChecks(t *testing.T) {
	rs, err := rules.NewRulesService()
	assert.NoError(t, err)

	byID := map[string]*rules.Rule{}
	for _, category := range models.CategoryTitles {
		for _, r := range rs.GetCategoryRules(category) {
			byID[r.ID] = r
		}
	}

	trivy := byID["trivy-rule"]
	assert.NotNil(t, trivy)
	assert.NotEmpty(t, trivy.Weakened)
	assert.True(t, trivy.Weakened[0].RunMatches.MatchString("trivy fs --exit-code 0 ."))
	assert.False(t, trivy.Weakened[0].RunMatches.MatchString("trivy fs --exit-code 1 ."))

	tfsec := byID["tfsec-rule"]
	assert.NotNil(t, tfsec)
	var hasInput bool
	for _, check := range tfsec.Weakened {
		if check.Input == "soft_fail" && check.Equals == "true" {
			hasInput = true
		}
	}
	assert.True(t, hasInput, "tfsec should be weakened by soft_fail: true")

	for _, id := range []string{"checkov-rule", "tfsec-rule"} {
		softFail := byID[id].Weakened[0].RunMatches
		assert.True(t, softFail.MatchString("checkov -d . --soft-fail"), id)
		assert.True(t, softFail.MatchString("checkov -d . --soft-fail --quiet"), id)
		assert.False(t, softFail.MatchString("checkov -d . --soft-fail-on CKV_AWS_20"), "%s: --soft-fail-on softens only the listed checks", id)
	}
}

func TestCustomRuleJoinsTheCatalogue(t *testing.T) {
	rs, err := rules.NewRulesService(rules.FileRule{
		ID:         "acme-scan",
		ApplyOn:    []string{rules.FieldJobRun},
		Categories: []string{"sast"},
		Regex:      `\bacme-scan\b`,
	})
	assert.NoError(t, err)

	var ids []string
	for _, r := range rs.GetCategoryRules(models.CategoryTitle("SAST")) {
		ids = append(ids, r.ID)
	}
	assert.Contains(t, ids, "acme-scan")
	assert.Contains(t, ids, "semgrep-rule")
}

func TestCustomRulesAreChecked(t *testing.T) {
	valid := func() rules.FileRule {
		return rules.FileRule{ID: "acme", ApplyOn: []string{rules.FieldJobRun}, Categories: []string{"SAST"}, Regex: "acme"}
	}
	tests := []struct {
		name     string
		edit     func(*rules.FileRule)
		contains string
	}{
		{"no id", func(r *rules.FileRule) { r.ID = "" }, "needs an id"},
		{"embedded id", func(r *rules.FileRule) { r.ID = "semgrep-rule" }, "already taken"},
		{"no regex", func(r *rules.FileRule) { r.Regex = "" }, "needs a regex"},
		{"bad regex", func(r *rules.FileRule) { r.Regex = "acme(" }, "regex"},
		{"no applyOn", func(r *rules.FileRule) { r.ApplyOn = nil }, "needs applyOn"},
		{"bad applyOn", func(r *rules.FileRule) { r.ApplyOn = []string{"Job.Steps"} }, "unknown applyOn"},
		{"no category", func(r *rules.FileRule) { r.Categories = nil }, "at least one category"},
		{"bad category", func(r *rules.FileRule) { r.Categories = []string{"Fuzzing"} }, "unknown category"},
		{"bad unless", func(r *rules.FileRule) { r.Unless = "--dry-run(" }, "unless"},
		{"unlessPackage without a name match", func(r *rules.FileRule) { r.UnlessPackage = "acme/action" }, "unlessPackage only qualifies"},
		{"bad unlessPackage", func(r *rules.FileRule) {
			r.ApplyOn = []string{rules.FieldArtifactName}
			r.UnlessPackage = "acme("
		}, "unlessPackage"},
		{"bad weakenedWhen", func(r *rules.FileRule) {
			r.Weakened = []rules.FileWeakenedWhen{{RunMatches: "--dry-run", Verdict: "broken"}}
		}, "verdict"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := valid()
			tt.edit(&r)
			_, err := rules.NewRulesService(r)
			assert.ErrorContains(t, err, tt.contains)
		})
	}
}

func TestCustomRuleIDsMustBeUnique(t *testing.T) {
	r := rules.FileRule{ID: "acme", ApplyOn: []string{rules.FieldJobRun}, Categories: []string{"SAST"}, Regex: "acme"}
	_, err := rules.NewRulesService(r, r)
	assert.ErrorContains(t, err, "already taken")
}
