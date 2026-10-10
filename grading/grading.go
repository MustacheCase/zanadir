// Package grading judges how much protection a control actually provides.
// Presence is not effectiveness: a scanner wired up so it can never fail a
// build leaves a repository worse off than one with no scanner, because
// everybody believes the control is there.
package grading

import (
	"strings"

	"github.com/MustacheCase/zanadir/matcher"
	"github.com/MustacheCase/zanadir/models"
	"github.com/MustacheCase/zanadir/rules"
)

type Verdict string

const (
	// Enforcing: the control can fail the build on the path that gates a change.
	Enforcing Verdict = "enforcing"
	// Partial: it can fail a build, but not on every relevant path.
	Partial Verdict = "partial"
	// Advisory: it runs, and cannot block anything.
	Advisory Verdict = "advisory"
	// Unknown: the configuration cannot be read well enough to say.
	Unknown Verdict = "unknown"
)

// Events that stand between a change and the default branch.
var gatingEvents = map[string]bool{
	"pull_request":        true,
	"pull_request_target": true,
	"merge_group":         true,
}

// Grade judges one matched control.
func Grade(finding *matcher.Finding) Verdict {
	if finding == nil || finding.Artifact == nil {
		return Unknown
	}

	if job := finding.Job; job != nil {
		if job.ContinueOnError {
			return Advisory
		}
		// A tool-specific flag is an explicit statement that this control
		// cannot fail, so it outranks a condition we cannot read.
		if verdict, found := toolSpecific(finding.Rule, job); found {
			return verdict
		}
		// An if: can reference arbitrary context. Reading one wrong is worse
		// than admitting it cannot be read, so no pattern is recognised yet.
		if job.If != "" || job.JobIf != "" {
			return Unknown
		}
	}

	return gradeTriggers(finding.Artifact)
}

// toolSpecific applies a rule's weakenedWhen checks: the flags that defeat one
// particular tool, which no generic signal can see.
func toolSpecific(rule *rules.Rule, job *models.Job) (Verdict, bool) {
	if rule == nil {
		return "", false
	}

	for _, check := range rule.Weakened {
		if check.RunMatches != nil {
			if job.Run != "" && check.RunMatches.MatchString(job.Run) {
				return Verdict(check.Verdict), true
			}
			continue
		}
		if value, ok := job.With[check.Input]; ok && strings.EqualFold(value, check.Equals) {
			return Verdict(check.Verdict), true
		}
	}
	return "", false
}

func gradeTriggers(artifact *models.Artifact) Verdict {
	// No triggers captured is not the same as a workflow that runs on nothing:
	// the GitLab and CircleCI parsers record none at all.
	if len(artifact.Triggers) == 0 {
		return Unknown
	}

	gates, runs := false, false
	for _, trigger := range artifact.Triggers {
		switch {
		case gatingEvents[trigger.Event]:
			gates = true
		case trigger.Event == "push":
			runs = true
		}
	}

	switch {
	case gates:
		return Enforcing
	case runs:
		return Partial
	default:
		// schedule, workflow_dispatch, release and the rest: the control runs,
		// but never between a change and the branch it lands on.
		return Advisory
	}
}

// Assessment is a category's verdict and the workflow the verdict came from,
// so a report can point at the configuration rather than guess at a location.
type Assessment struct {
	Verdict  Verdict
	Location string
}

// strength orders verdicts. Unknown sits above the verdicts that get
// reported so that one unreadable control keeps a category quiet: saying
// "weakened" when the answer is "cannot tell" is the false positive this
// feature can least afford.
var strength = map[Verdict]int{
	Advisory:  0,
	Partial:   1,
	Unknown:   2,
	Enforcing: 3,
}

// stronger reports whether a is the better verdict of the two.
func stronger(a, b Verdict) bool {
	return strength[a] > strength[b]
}

// Weakened reports whether a verdict means the control protects less than its
// presence suggests. Unknown is deliberately excluded: not knowing is not a
// finding.
func Weakened(verdict Verdict) bool {
	return verdict == Advisory || verdict == Partial
}

// ByCategory grades every finding and keeps the strongest verdict for each
// category: one enforcing control is enough, however many weak ones sit
// beside it.
func ByCategory(findings []*matcher.Finding) map[string]Assessment {
	best := make(map[string]Assessment)
	for _, finding := range findings {
		if finding == nil {
			continue
		}
		candidate := Assessment{Verdict: Grade(finding), Location: finding.Location}
		if current, seen := best[finding.Category]; seen && !stronger(candidate.Verdict, current.Verdict) {
			continue
		}
		best[finding.Category] = candidate
	}
	return best
}
