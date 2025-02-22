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
	switch field {
	case "Artifact.Name":
		return rule.Regex.MatchString(artifact.Name)
	case "Job.Package":
		for _, job := range artifact.Jobs {
			if rule.Regex.MatchString(job.Package) {
				return true
			}
		}
	}
	return false
}

func NewMatchService() Matcher {
	return &service{}
}
