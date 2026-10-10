// Package grading judges how much protection a control actually provides.
// Presence is not effectiveness: a scanner wired up so it can never fail a
// build leaves a repository worse off than one with no scanner, because
// everybody believes the control is there.
package grading

import (
	"github.com/MustacheCase/zanadir/matcher"
	"github.com/MustacheCase/zanadir/models"
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
		// An if: can reference arbitrary context. Reading one wrong is worse
		// than admitting it cannot be read, so no pattern is recognised yet.
		if job.If != "" || job.JobIf != "" {
			return Unknown
		}
	}

	return gradeTriggers(finding.Artifact)
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

// strength orders verdicts for Best. Unknown sits above the verdicts that get
// reported so that one unreadable control keeps a category quiet: saying
// "weakened" when the answer is "cannot tell" is the false positive this
// feature can least afford.
var strength = map[Verdict]int{
	Advisory:  0,
	Partial:   1,
	Unknown:   2,
	Enforcing: 3,
}

// Best returns the strongest verdict among a category's controls. One
// enforcing control is enough, however many weak ones sit beside it.
func Best(verdicts []Verdict) Verdict {
	if len(verdicts) == 0 {
		return Unknown
	}
	best := verdicts[0]
	for _, v := range verdicts[1:] {
		if strength[v] > strength[best] {
			best = v
		}
	}
	return best
}

// Weakened reports whether a verdict means the control protects less than its
// presence suggests. Unknown is deliberately excluded: not knowing is not a
// finding.
func Weakened(verdict Verdict) bool {
	return verdict == Advisory || verdict == Partial
}

// ByCategory grades every finding and returns the strongest verdict for each
// category.
func ByCategory(findings []*matcher.Finding) map[string]Verdict {
	verdicts := make(map[string][]Verdict)
	for _, finding := range findings {
		if finding == nil {
			continue
		}
		verdicts[finding.Category] = append(verdicts[finding.Category], Grade(finding))
	}

	best := make(map[string]Verdict, len(verdicts))
	for category, list := range verdicts {
		best[category] = Best(list)
	}
	return best
}
