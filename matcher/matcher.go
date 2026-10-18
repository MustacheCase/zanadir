package matcher

import (
	"regexp"
	"strings"

	"github.com/MustacheCase/zanadir/internal/shell"
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
			for _, job := range matchingJobs(artifact, rule) {
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

	return findings
}

// matchingJobs returns every step a rule matches. A match on the workflow's
// name belongs to no step and yields a single nil, but only when no step
// matches: a step says how the tool is wired, which grading needs, while a
// name match would outrank it ungraded.
func matchingJobs(artifact *models.Artifact, rule *rules.Rule) []*models.Job {
	var matched []*models.Job
	named := false
	for _, field := range rule.ApplyOn {
		if field == rules.FieldArtifactName {
			named = named || rule.Regex.MatchString(artifact.Name)
			continue
		}
		matched = append(matched, matchingSteps(artifact, rule, field)...)
	}
	if named && len(matched) == 0 {
		return []*models.Job{nil}
	}
	return matched
}

func matchingSteps(artifact *models.Artifact, rule *rules.Rule, field string) []*models.Job {
	var matched []*models.Job
	for _, job := range artifact.Jobs {
		var text string
		switch field {
		case rules.FieldJobName:
			text = job.Name
		case rules.FieldJobPackage:
			if !setupAction.MatchString(job.Package) {
				text = job.Package
			}
		case rules.FieldJobRun:
			text = invocations(job.Run)
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

// setupAction names an action that installs a tool for later steps.
var setupAction = regexp.MustCompile(`(?i)(?:^|/)(?:setup|install|download)(?:-[^/]*)?$|-(?:installer|setup)$`)

// invocations keeps the commands of a run script that run something, one per
// line.
func invocations(run string) string {
	var kept []string
	for _, command := range shell.Commands(run) {
		if words := shell.Words(command); len(words) > 0 && !shell.Passive(words) {
			kept = append(kept, command)
		}
	}
	return strings.Join(kept, "\n")
}

func NewMatchService() Matcher {
	return &service{}
}
