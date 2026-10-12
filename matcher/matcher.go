package matcher

import (
	"github.com/MustacheCase/zanadir/models"
	"github.com/MustacheCase/zanadir/rules"
)

type Finding struct {
	Category string
	RuleID   string
	Location string
	// Job is nil when the match was on the workflow name.
	Artifact *models.Artifact
	Job      *models.Job
	Rule     *rules.Rule
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
				for _, job := range matchingJobs(artifact, rule, applyField) {
					for _, c := range rule.Categories {
						findings = append(findings, &Finding{
							Category: c,
							RuleID:   rule.ID,
							Location: artifact.Location,
							Artifact: artifact,
							Job:      job,
							Rule:     rule,
						})
					}
				}
			}
		}
	}

	return findings
}

// matchingJobs returns every step a rule matches. A match on the workflow name
// belongs to no step and yields a single nil.
func matchingJobs(artifact *models.Artifact, rule *rules.Rule, field string) []*models.Job {
	if field == rules.FieldArtifactName {
		if rule.Regex.MatchString(artifact.Name) {
			return []*models.Job{nil}
		}
		return nil
	}

	var matched []*models.Job
	for _, job := range artifact.Jobs {
		var text string
		switch field {
		case rules.FieldJobName:
			text = job.Name
		case rules.FieldJobPackage:
			text = job.Package
		case rules.FieldJobRun:
			text = job.Run
		default:
			continue
		}
		// A rule that can match "" would otherwise match every step that
		// lacks the field.
		if text != "" && rule.Regex.MatchString(text) {
			matched = append(matched, job)
		}
	}
	return matched
}

func NewMatchService() Matcher {
	return &service{}
}
