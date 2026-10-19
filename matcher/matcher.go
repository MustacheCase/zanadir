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
			named = named || matches(rule, artifact.Name)
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
			text = invocations(job.Run, rule.Unless)
		default:
			continue
		}
		// A rule that can match "" would otherwise match every step that
		// lacks the field. A run's excepted commands are already dropped.
		if text != "" && (field == rules.FieldJobRun && rule.Regex.MatchString(text) || matches(rule, text)) {
			matched = append(matched, job)
		}
	}
	return matched
}

func matches(rule *rules.Rule, text string) bool {
	return rule.Regex.MatchString(text) && (rule.Unless == nil || !rule.Unless.MatchString(text))
}

// setupAction names an action that installs a tool for later steps.
var setupAction = regexp.MustCompile(`(?i)(?:^|/)(?:setup|install|download)(?:-[^/]*)?$|-(?:installer|setup)$`)

// invocations keeps the commands of a run script that run something, one per
// line, less those a rule excepts.
func invocations(run string, unless *regexp.Regexp) string {
	var kept []string
	eachInvocation(run, func(_, invoked string) {
		if unless == nil || !unless.MatchString(invoked) {
			kept = append(kept, invoked)
		}
	})
	return strings.Join(kept, "\n")
}

// Commands returns the commands of a run script that the rule matches, as
// written, so a flag is read off the command that runs the tool rather than
// off a neighbour.
func Commands(run string, rule *rules.Rule) []string {
	var matched []string
	eachInvocation(run, func(command, invoked string) {
		if matches(rule, invoked) {
			matched = append(matched, command)
		}
	})
	return matched
}

func eachInvocation(run string, visit func(command, invoked string)) {
	for _, command := range shell.Commands(run) {
		if words := shell.Words(command); len(words) > 0 && !shell.Passive(words) {
			visit(command, strings.Join(shell.Invoked(words), " "))
		}
	}
}

func NewMatchService() Matcher {
	return &service{}
}
