package matcher

import (
	"github.com/MustacheCase/zanadir/models"
	"github.com/MustacheCase/zanadir/rules"
)

type Finding struct {
	Category string
	RuleID   string
	Location string
}

type Matcher interface {
	Match([]*models.Artifact, []*rules.Rule) []*Finding
}

type service struct{}

func (s *service) Match(artifacts []*models.Artifact, ruleSet []*rules.Rule) []*Finding {
	var findings []*Finding

	for _, rule := range ruleSet {
		for _, artifact := range artifacts {
			for _, applyField := range rule.ApplyOn {
				if matchesRule(artifact, rule, applyField) {
					for _, c := range rule.Categories {
						findings = append(findings, &Finding{
							Category: c,
							RuleID:   rule.ID,
							Location: artifact.Location,
						})
					}
				}
			}
		}
	}

	return findings
}

func matchesRule(artifact *models.Artifact, rule *rules.Rule, field string) bool {
	check := func(value string) bool {
		return rule.Regex.MatchString(value)
	}

	switch field {
	case "Artifact.Name":
		return check(artifact.Name)
	case "Job.Package":
		return anyMatch(artifact.Jobs, func(job *models.Job) bool { return check(job.Package) })
	case "Job.Script":
		return anyMatch(artifact.Jobs, func(job *models.Job) bool { return check(job.Script) })
	}

	return false
}

// anyMatch checks if any element in a slice satisfies the predicate
func anyMatch[T any](items []T, predicate func(T) bool) bool {
	for _, item := range items {
		if predicate(item) {
			return true
		}
	}
	return false
}

func NewMatchService() Matcher {
	return &service{}
}
