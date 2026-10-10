package matcher

import (
	"github.com/MustacheCase/zanadir/models"
	"github.com/MustacheCase/zanadir/rules"
)

type Finding struct {
	Category string
	RuleID   string
	Location string
	// Artifact is the workflow the rule matched and Job the step inside it.
	// Grading needs both. Job is nil when the match was on the workflow name,
	// which belongs to no step.
	Artifact *models.Artifact
	Job      *models.Job
	// Rule is what matched, so grading can read the tool-specific ways this
	// particular tool can be configured not to fail a build.
	Rule *rules.Rule
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

// matchingJobs returns every step a rule matches, not just the first: the same
// tool can be enforcing in one job and unable to fail the build in another,
// and a category is only as weak as its strongest control.
//
// A match on the workflow name belongs to no step, so it yields a single nil
// and grading has only the workflow's triggers to go on.
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
		// Guarding the empty string keeps a rule that can match it from
		// matching every step that lacks the field. No shipped rule can.
		if text != "" && rule.Regex.MatchString(text) {
			matched = append(matched, job)
		}
	}
	return matched
}

func NewMatchService() Matcher {
	return &service{}
}
