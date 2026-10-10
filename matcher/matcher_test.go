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
			Artifact: artifacts[0],
			// A workflow-name match belongs to no step.
			Job: nil,
		},
		{
			Category: "CategoryB",
			RuleID:   "rule2",
			Location: "path/to/artifact2",
			Artifact: artifacts[1],
			Job:      artifacts[1].Jobs[0],
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

// The same tool can be wired up properly in one job and unable to fail the
// build in another. Reporting only the first match would decide a category's
// verdict on whichever step happened to be parsed first.
func TestMatchReportsEveryMatchingStep(t *testing.T) {
	artifact := &models.Artifact{
		Name:     "ci",
		Location: ".github/workflows/ci.yml",
		Jobs: []*models.Job{
			{Name: "scan", Package: "gitleaks/gitleaks-action", ContinueOnError: true},
			{Name: "unrelated", Package: "actions/checkout"},
			{Name: "gate", Package: "gitleaks/gitleaks-action"},
		},
	}
	rule := &rules.Rule{
		ID:         "gitleaks-rule",
		ApplyOn:    []string{rules.FieldJobPackage},
		Categories: []string{"Secrets Detection"},
		Regex:      regexp.MustCompile(`(?i)\bgitleaks\b`),
	}

	findings := matcher.NewMatchService().Match([]*models.Artifact{artifact}, []*rules.Rule{rule})

	assert.Len(t, findings, 2)
	assert.Same(t, artifact.Jobs[0], findings[0].Job)
	assert.Same(t, artifact.Jobs[2], findings[1].Job)
}

// A rule that can match the empty string would otherwise match every step
// that lacks the field it applies to.
func TestMatchIgnoresEmptyFields(t *testing.T) {
	artifact := &models.Artifact{
		Name:     "ci",
		Location: ".github/workflows/ci.yml",
		Jobs:     []*models.Job{{Name: "a"}, {Name: "b", Package: "actions/checkout"}},
	}
	rule := &rules.Rule{
		ID:         "greedy",
		ApplyOn:    []string{rules.FieldJobPackage},
		Categories: []string{"SCA"},
		Regex:      regexp.MustCompile(`^actions/checkout$|^$`),
	}

	findings := matcher.NewMatchService().Match([]*models.Artifact{artifact}, []*rules.Rule{rule})

	assert.Len(t, findings, 1)
	assert.Same(t, artifact.Jobs[1], findings[0].Job)
}
