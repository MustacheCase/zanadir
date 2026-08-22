package matcher_test

import (
	"regexp"
	"testing"

	"github.com/MustacheCase/zanadir/matcher"
	"github.com/MustacheCase/zanadir/models"
	"github.com/MustacheCase/zanadir/rules"
	"github.com/stretchr/testify/assert"
)

func TestMatch(t *testing.T) {
	svc := matcher.NewMatchService()

	artifacts := []*models.Artifact{
		{
			Name:     "artifact1",
			Location: "path/to/artifact1",
			Jobs: []*models.Job{
				{Package: "package1"},
			},
		},
		{
			Name:     "artifact2",
			Location: "path/to/artifact2",
			Jobs: []*models.Job{
				{Package: "package2"},
			},
		},
	}

	ruleSet := []*rules.Rule{
		{
			ID:         "rule1",
			Regex:      regexp.MustCompile("artifact1"),
			ApplyOn:    []string{"Artifact.Name"},
			Categories: []string{"CategoryA"},
		},
		{
			ID:         "rule2",
			Regex:      regexp.MustCompile("package2"),
			ApplyOn:    []string{"Job.Package"},
			Categories: []string{"CategoryB"},
		},
	}

	findings := svc.Match(artifacts, ruleSet)

	expectedFindings := []*matcher.Finding{
		{
			Category: "CategoryA",
			RuleID:   "rule1",
			Location: "path/to/artifact1",
		},
		{
			Category: "CategoryB",
			RuleID:   "rule2",
			Location: "path/to/artifact2",
		},
	}

	assert.Equal(t, expectedFindings, findings)
}

func TestMatch_NoMatches(t *testing.T) {
	svc := matcher.NewMatchService()

	artifacts := []*models.Artifact{
		{
			Name:     "artifact3",
			Location: "path/to/artifact3",
			Jobs: []*models.Job{
				{Package: "package3"},
			},
		},
	}

	ruleSet := []*rules.Rule{
		{
			ID:         "rule3",
			Regex:      regexp.MustCompile("artifactX"),
			ApplyOn:    []string{"Artifact.Name"},
			Categories: []string{"CategoryX"},
		},
	}

	findings := svc.Match(artifacts, ruleSet)

	assert.Empty(t, findings)
}

// Tools invoked from a shell step were invisible before Job.Run existed.
func TestMatchRunCommands(t *testing.T) {
	m := matcher.NewMatchService()
	rule := &rules.Rule{
		ID:         "trivy-rule",
		ApplyOn:    []string{rules.FieldJobRun},
		Categories: []string{"SCA"},
		Regex:      regexp.MustCompile("(?i)trivy"),
	}

	artifacts := []*models.Artifact{{
		Name:     "ci",
		Location: ".github/workflows/ci.yml",
		Jobs:     []*models.Job{{Name: "security", Run: "trivy fs --exit-code 1 ."}},
	}}

	findings := m.Match(artifacts, []*rules.Rule{rule})
	assert.Len(t, findings, 1)
	assert.Equal(t, "SCA", findings[0].Category)
}

// unit-tests.yaml declared Job.Name but matchesRule had no case for it.
func TestMatchJobName(t *testing.T) {
	m := matcher.NewMatchService()
	rule := &rules.Rule{
		ID:         "unit-test-rule",
		ApplyOn:    []string{rules.FieldJobName},
		Categories: []string{"Unit Tests"},
		Regex:      regexp.MustCompile("(?i).*test.*"),
	}

	artifacts := []*models.Artifact{{
		Name: "pipeline",
		Jobs: []*models.Job{{Name: "run-tests"}},
	}}

	findings := m.Match(artifacts, []*rules.Rule{rule})
	assert.Len(t, findings, 1)
	assert.Equal(t, "Unit Tests", findings[0].Category)
}

func TestMatchEmptyRunIsNotMatched(t *testing.T) {
	m := matcher.NewMatchService()
	rule := &rules.Rule{
		ID:         "match-anything",
		ApplyOn:    []string{rules.FieldJobRun},
		Categories: []string{"SCA"},
		Regex:      regexp.MustCompile("(?i).*"),
	}

	artifacts := []*models.Artifact{{
		Name: "ci",
		Jobs: []*models.Job{{Name: "build", Package: "actions/checkout"}},
	}}

	assert.Empty(t, m.Match(artifacts, []*rules.Rule{rule}))
}
