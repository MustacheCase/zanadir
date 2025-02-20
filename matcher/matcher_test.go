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
