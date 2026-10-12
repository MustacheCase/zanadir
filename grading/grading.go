// Package grading judges whether a control can actually fail a build, rather
// than only whether it is present.
package grading

import (
	"strings"

	"github.com/MustacheCase/zanadir/matcher"
	"github.com/MustacheCase/zanadir/models"
	"github.com/MustacheCase/zanadir/rules"
)

type Verdict string

const (
	Enforcing Verdict = "enforcing"
	Partial   Verdict = "partial"
	Advisory  Verdict = "advisory"
	Unknown   Verdict = "unknown"
)

// Assessment is a category's verdict and where the evidence for it sits.
type Assessment struct {
	Verdict  Verdict
	Location string
}

var gatingEvents = map[string]bool{
	"pull_request":        true,
	"pull_request_target": true,
	"merge_group":         true,
}

// Unknown outranks the reported verdicts, so one unreadable control keeps a
// category quiet.
var strength = map[Verdict]int{
	Advisory:  0,
	Partial:   1,
	Unknown:   2,
	Enforcing: 3,
}

// Grade judges one matched control.
func Grade(finding *matcher.Finding) Verdict {
	if finding == nil || finding.Artifact == nil {
		return Unknown
	}
	if finding.Artifact.OutsidePipeline {
		return Advisory
	}

	if job := finding.Job; job != nil {
		if job.ContinueOnError || job.Manual {
			return Advisory
		}
		if verdict, found := toolSpecific(finding.Rule, job); found {
			return verdict
		}
		// An if: can reference arbitrary context, so it is never interpreted.
		if job.If != "" || job.JobIf != "" {
			return Unknown
		}
	}

	return gradeTriggers(finding.Artifact)
}

// toolSpecific applies a rule's weakenedWhen checks.
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
	// No triggers captured is not a workflow that runs on nothing: the GitLab
	// and CircleCI parsers record none at all.
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
		return Advisory
	}
}

func stronger(a, b Verdict) bool {
	return strength[a] > strength[b]
}

// Worse reports whether a protects less than b.
func Worse(a, b Verdict) bool {
	return strength[a] < strength[b]
}

// Weakened reports whether a verdict is worth reporting as a weakness. Unknown
// is excluded: not knowing is not a finding.
func Weakened(verdict Verdict) bool {
	return verdict == Advisory || verdict == Partial
}

// ByCategory keeps the strongest verdict for each category.
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
