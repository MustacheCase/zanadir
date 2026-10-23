// Package matcher finds which rules match the jobs of parsed CI configuration.
package matcher

import (
	"regexp"
	"slices"
	"strings"

	"github.com/MustacheCase/zanadir/internal/shell"
	"github.com/MustacheCase/zanadir/models"
	"github.com/MustacheCase/zanadir/rules"
)

// Finding is one rule matched in one place.
type Finding struct {
	Category string
	RuleID   string
	Location string
	// Job is nil when the match was on the workflow name.
	Artifact *models.Artifact
	Job      *models.Job
	Rule     *rules.Rule
}

// Matcher finds the rules that match parsed CI configuration.
type Matcher interface {
	Match([]*models.Artifact, []*rules.Rule) []*Finding
}

type service struct{}

func (s *service) Match(artifacts []*models.Artifact, ruleSet []*rules.Rule) []*Finding {
	var findings []*Finding
	runs := scripts{}

	for _, rule := range ruleSet {
		for _, artifact := range artifacts {
			for _, job := range runs.matchingJobs(artifact, rule) {
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
func (runs scripts) matchingJobs(artifact *models.Artifact, rule *rules.Rule) []*models.Job {
	var matched []*models.Job
	named := false
	for _, field := range rule.ApplyOn {
		if field == rules.FieldArtifactName {
			named = named || matches(rule, artifact.Name) && !packageDecides(rule, artifact)
			continue
		}
		matched = append(matched, runs.matchingSteps(artifact, rule, field)...)
	}
	if named && len(matched) == 0 {
		return []*models.Job{nil}
	}
	return matched
}

func (runs scripts) matchingSteps(artifact *models.Artifact, rule *rules.Rule, field string) []*models.Job {
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
			text = runs.invocations(job.Run, rule.Unless)
		default:
			continue
		}
		// A rule that can match "" would otherwise match every step that
		// lacks the field. A run's excepted commands are already dropped.
		if text != "" && (field == rules.FieldJobRun && runs.mayMatch(rule, job.Run) && rule.Regex.MatchString(text) || field != rules.FieldJobRun && matches(rule, text)) {
			matched = append(matched, job)
		}
	}
	return matched
}

// packageDecides reports whether a step's package rules a name match out.
func packageDecides(rule *rules.Rule, artifact *models.Artifact) bool {
	return rule.UnlessPackage != nil && slices.ContainsFunc(artifact.Jobs, func(job *models.Job) bool {
		return rule.UnlessPackage.MatchString(job.Package)
	})
}

func matches(rule *rules.Rule, text string) bool {
	return mayMatch(rule, strings.ToLower(text)) && rule.Regex.MatchString(text) && (rule.Unless == nil || !rule.Unless.MatchString(text))
}

// mayMatch is false when lowered holds none of the literals every match of
// the rule contains, which is far cheaper to find than to run the regex.
func mayMatch(rule *rules.Rule, lowered string) bool {
	return rule.Literals == nil || slices.ContainsFunc(rule.Literals, func(literal string) bool {
		return strings.Contains(lowered, literal)
	})
}

// setupAction names an action that installs a tool for later steps.
var setupAction = regexp.MustCompile(`(?i)(?:^|/)(?:setup|install|download)(?:-[^/]*)?$|-(?:installer|setup)$`)

// scripts parses each run script once for every rule: a script is split into
// commands and words, which cost far more than the rules' regexes.
type scripts map[string]*script

type script struct {
	invoked []string
	joined  string
	lowered string
}

func (runs scripts) of(run string) *script {
	if parsed, ok := runs[run]; ok {
		return parsed
	}
	parsed := &script{}
	eachInvocation(run, func(_, invoked string) { parsed.invoked = append(parsed.invoked, invoked) })
	parsed.joined = strings.Join(parsed.invoked, "\n")
	parsed.lowered = strings.ToLower(parsed.joined)
	runs[run] = parsed
	return parsed
}

func (runs scripts) mayMatch(rule *rules.Rule, run string) bool {
	return mayMatch(rule, runs.of(run).lowered)
}

// invocations keeps the commands of a run script that run something, one per
// line, less those a rule excepts.
func (runs scripts) invocations(run string, unless *regexp.Regexp) string {
	parsed := runs.of(run)
	if unless == nil {
		return parsed.joined
	}
	var kept []string
	for _, invoked := range parsed.invoked {
		if !unless.MatchString(invoked) {
			kept = append(kept, invoked)
		}
	}
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

// NewMatchService returns the default Matcher.
func NewMatchService() Matcher {
	return &service{}
}
