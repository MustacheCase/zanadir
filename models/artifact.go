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
	Languages   []string
	// Disabled is a workflow GitHub has turned off and nothing enabled calls,
	// so it never runs.
	Disabled bool
}

// DisabledWorkflowsFile lists the workflows a fetched repository has turned
// off, one path a line. Only GitHub's API says so, not the files.
const DisabledWorkflowsFile = ".git/zanadir-disabled-workflows"

// CodeScanningFile marks a fetched repository whose code scanning default
// setup is on, listing the languages it analyses one a line.
const CodeScanningFile = ".git/zanadir-code-scanning"

// DependabotSecurityUpdatesFile marks a fetched repository with security updates
// enabled without a Dependabot configuration, listing package-ecosystem names
// as used in dependabot.yml, one a line.
// An empty file means the ecosystems could not be determined.
const DependabotSecurityUpdatesFile = ".git/zanadir-dependabot-security-updates"

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
	// WorkingDirectory is where Run starts, relative to the repository root.
	// Empty is the root.
	WorkingDirectory string
	// Followed are the commands in Run that stand for a package script or
	// build target, and the lines each one expanded to. Rules drop a wrapper
	// only when its expansion matches that rule; otherwise the wrapper text
	// can still prove that the job runs the tool.
	Followed []FollowedCommand
	// ContinueOnError covers GitLab's allow_failure too.
	ContinueOnError bool
	// Manual runs only when somebody asks.
	Manual bool
	// If is the step's condition, JobIf the enclosing job's.
	If    string
	JobIf string
	Needs []string
	With  map[string]string
	// Env is the step's environment: the workflow's, job's and step's env:.
	Env map[string]string
	// Check is the name a GitHub Actions job reports its result under: its
	// name:, or its id. It may hold an unevaluated ${{ }} expression.
	Check string
}

// FollowedCommand pairs a wrapper invocation with the commands it expands to.
type FollowedCommand struct {
	Command   string
	Expansion []string
}

// RuleRunFor removes wrapper commands only when the supplied rule also matches
// their expansion. This keeps wrapper-based evidence (for example npm test)
// when its script body does not itself identify a recognised test command.
func (j *Job) RuleRunFor(matchesExpansion func(string) bool) string {
	run := j.Run
	for _, followed := range j.Followed {
		if matchesExpansion != nil {
			matches := false
			for _, line := range followed.Expansion {
				if matchesExpansion(line) {
					matches = true
					break
				}
			}
			if !matches {
				continue
			}
		}
		// Only a whole command is removed: "npm run lint" is not the start
		// of "npm run lint:fix". The boundaries on either side are kept.
		loc := followedPattern(followed.Command).FindStringSubmatchIndex(run)
		if loc == nil {
			continue
		}
		run = run[:loc[0]] + run[loc[2]:loc[3]] + run[loc[4]:loc[5]] + run[loc[1]:]
	}
	return run
}

// followedPattern matches command as a whole command within a run.
func followedPattern(command string) *regexp.Regexp {
	return regexp.MustCompile(`(^|[\s;&|(])` + regexp.QuoteMeta(command) + `($|[\s;&|)])`)
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
