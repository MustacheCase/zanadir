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
}
