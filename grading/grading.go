// Package grading judges whether a control can actually fail a build, rather
// than only whether it is present.
package grading

import (
	"slices"
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
	// Reason explains a verdict the workflow alone does not show.
	Reason string
}

// Required reports whether a job must succeed for a change to merge; nil when
// the repository's merge settings were not read.
type Required func(job *models.Job) bool

// NotRequired is the reason a control that runs on pull requests is partial.
const NotRequired = "its check is not required to merge"

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

	shipping := finding.Category == string(models.SupplyChain)
	if job := finding.Job; job != nil {
		if job.ContinueOnError {
			return Advisory
		}
		// A manual step may be the release itself; the file cannot say.
		if job.Manual && shipping {
			return Unknown
		}
		if job.Manual {
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

	if shipping {
		return gradeShipping(finding.Artifact)
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

// Supply chain tools record and sign what ships rather than look for
// something to block, so their path is the one that publishes: a release, a
// push, a tag or a workflow chained after one. Running there is enforcing. A
// manual run may be the release, which the file cannot tell, so it is
// unknown; only a schedule is detached from what ships.
var shippingEvents = map[string]bool{
	"release":      true,
	"push":         true,
	"tag":          true,
	"workflow_run": true,
}

func gradeShipping(artifact *models.Artifact) Verdict {
	if len(artifact.Triggers) == 0 {
		return Unknown
	}
	verdict := Advisory
	for _, trigger := range artifact.Triggers {
		switch {
		case shippingEvents[trigger.Event] || gatingEvents[trigger.Event]:
			return Enforcing
		case trigger.Event == "workflow_dispatch":
			verdict = Unknown
		}
	}
	return verdict
}

// gated holds a control that runs on pull requests to the merge settings: it
// blocks a merge only if a check of its job is required. A match on the
// workflow's name stands for every job in it.
func gated(a Assessment, finding *matcher.Finding, required Required) Assessment {
	if required == nil || a.Verdict != Enforcing || finding.Category == string(models.SupplyChain) {
		return a
	}
	jobs := finding.Artifact.Jobs
	if finding.Job != nil {
		jobs = []*models.Job{finding.Job}
	}
	checked := false
	for _, job := range jobs {
		if job.Check == "" {
			continue
		}
		if blocks(job, finding.Artifact.Jobs, required, map[string]bool{}) {
			return a
		}
		checked = true
	}
	if checked {
		a.Verdict, a.Reason = Partial, NotRequired
	}
	return a
}

// blocks reports whether a job holds back a merge itself or through a job
// that needs it: a required gate job that waits on the real ones and fails
// unless they all succeeded.
func blocks(job *models.Job, jobs []*models.Job, required Required, seen map[string]bool) bool {
	if required(job) {
		return true
	}
	seen[job.Name] = true
	for _, other := range jobs {
		if !seen[other.Name] && slices.Contains(other.Needs, job.Name) && blocks(other, jobs, required, seen) {
			return true
		}
	}
	return false
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
func ByCategory(findings []*matcher.Finding, required Required) map[string]Assessment {
	best := make(map[string]Assessment)
	for _, finding := range findings {
		if finding == nil {
			continue
		}
		candidate := gated(Assessment{Verdict: Grade(finding), Location: finding.Location}, finding, required)
		if current, seen := best[finding.Category]; seen && !stronger(candidate.Verdict, current.Verdict) {
			continue
		}
		best[finding.Category] = candidate
	}
	return best
}
