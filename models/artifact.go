// Package models holds the types shared across zanadir: parsed CI configuration, categories and errors.
package models

import (
	"regexp"
	"strings"
)

// Artifact is one CI configuration file as parsed: its triggers and jobs.
type Artifact struct {
	Name string
	// Empty means none were captured, not that it runs on everything.
	Triggers []Trigger
	Jobs     []*Job
	Location string
	// Declared in configuration rather than a CI job, such as Dependabot.
	OutsidePipeline bool
	// Skipped says why the file was not read; it then has no jobs.
	Skipped string
	// DefaultBranch is the repository's, when it could be read.
	DefaultBranch string
	// ToolConfigs are the config files named by rules that the repository has.
	ToolConfigs map[string]bool
}

// Trigger is one event a CI configuration runs on, with its filters.
type Trigger struct {
	Event    string
	Branches []string
	// IgnoredBranches and Types narrow a GitHub event: branches-ignore and,
	// for pull requests, which activity types it runs on.
	IgnoredBranches []string
	Types           []string
}

// Job is one step of CI configuration, with what decides whether its failure fails the build.
type Job struct {
	Name    string
	Package string
	Version string
	// Run is the shell command of a step that invokes a tool directly.
	Run string
	// ContinueOnError covers GitLab's allow_failure too.
	ContinueOnError bool
	// Manual runs only when somebody asks.
	Manual bool
	// If is the step's condition, JobIf the enclosing job's.
	If    string
	JobIf string
	Needs []string
	With  map[string]string
	// Check is the name a GitHub Actions job reports its result under: its
	// name:, or its id. It may hold an unevaluated ${{ }} expression.
	Check string
}

// HasTrigger reports whether the configuration runs on event.
func (a *Artifact) HasTrigger(event string) bool {
	for _, t := range a.Triggers {
		if t.Event == event {
			return true
		}
	}
	return false
}

var checkExpression = regexp.MustCompile(`\$\{\{.*?\}\}`)

// UnnamedCheck reports whether a job's check name is set entirely by
// expressions, such as ${{ matrix.tool }}, so which check it reports cannot
// be known.
func (j *Job) UnnamedCheck() bool {
	return j.Check != "" && strings.TrimSpace(checkExpression.ReplaceAllString(j.Check, "")) == ""
}

// CallsWorkflow reports whether a job runs a reusable workflow, whose jobs
// report their checks as "job / called job".
func (j *Job) CallsWorkflow() bool {
	return strings.Contains(j.Package, ".github/workflows/")
}
