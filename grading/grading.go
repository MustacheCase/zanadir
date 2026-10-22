// Package grading judges whether a control can actually fail a build, rather
// than only whether it is present.
package grading

import (
	"regexp"
	"slices"
	"strings"

	"github.com/MustacheCase/zanadir/internal/shell"
	"github.com/MustacheCase/zanadir/matcher"
	"github.com/MustacheCase/zanadir/models"
	"github.com/MustacheCase/zanadir/rules"
)

// Verdict says whether a control can fail a build.
type Verdict string

// Verdicts, strongest first.
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
		// The shell's way of saying continue-on-error: "|| true", "if ! …".
		if job.ContinueOnError || finding.Rule != nil && finding.Rule.Regex != nil && shell.Swallowed(job.Run, finding.Rule.Regex.MatchString) {
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

	// A match on the workflow's name cannot say which step runs the tool, but
	// a workflow whose every step may fail cannot fail at all.
	if finding.Job == nil && len(finding.Artifact.Jobs) > 0 &&
		!slices.ContainsFunc(finding.Artifact.Jobs, func(job *models.Job) bool { return !job.ContinueOnError }) {
		return Advisory
	}
	if shipping {
		return gradeShipping(finding.Artifact)
	}
	return gradeTriggers(finding.Artifact)
}

// runtimeExpression is GitHub's ${{ }} or Azure's $( ) and $[ ].
var runtimeExpression = regexp.MustCompile(`\$\{\{|\$\(|\$\[`)

// toolSpecific applies a rule's weakenedWhen checks.
func toolSpecific(rule *rules.Rule, job *models.Job) (Verdict, bool) {
	if rule == nil {
		return "", false
	}

	// A step matched by its name or action, with no command of the tool's
	// own, is read whole.
	commands := []string{job.Run}
	if rule.Regex != nil {
		if matched := matcher.Commands(job.Run, rule); len(matched) > 0 {
			commands = matched
		}
	}
	for _, check := range rule.Weakened {
		if check.RunMatches != nil {
			if job.Run != "" && slices.ContainsFunc(commands, check.RunMatches.MatchString) {
				return Verdict(check.Verdict), true
			}
			continue
		}
		value, ok := job.With[check.Input]
		switch {
		case ok && strings.EqualFold(value, check.Equals):
			return Verdict(check.Verdict), true
		// An input set by an expression may be the weakening value or not.
		case ok && runtimeExpression.MatchString(value):
			return Unknown, true
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

	gates, unclear, runs := false, false, false
	for _, trigger := range artifact.Triggers {
		switch {
		case !gatingEvents[trigger.Event] && trigger.Event == "push":
			runs = true
		case !gatingEvents[trigger.Event]:
		case closedOnly(trigger):
			runs = true
		case !coversDefaultBranch(trigger, artifact.DefaultBranch):
			unclear = true
		default:
			gates = true
		}
	}

	switch {
	case gates:
		return Enforcing
	case unclear:
		return Unknown
	case runs:
		return Partial
	default:
		return Advisory
	}
}

// reviewTypes are the pull request activity types that run on the changes
// themselves; GitHub's default is all three. A trigger on closed alone runs
// once the change has merged, like a push.
var reviewTypes = []string{"opened", "synchronize", "reopened"}

func closedOnly(trigger models.Trigger) bool {
	return trigger.Event != "merge_group" && len(trigger.Types) > 0 &&
		!slices.ContainsFunc(trigger.Types, func(t string) bool { return slices.Contains(reviewTypes, t) })
}

// defaultBranches stand for the default branch when it could not be read:
// the names it takes in practice (cli/cli and kafka use trunk).
var defaultBranches = []string{"main", "master", "trunk", "develop"}

// coversDefaultBranch reports whether a pull request trigger's branch filters
// admit the default branch. Unread, it is one of defaultBranches, or only
// those the filters name, if they name any; a filter that leaves them all
// out may cover the default branch or may not.
func coversDefaultBranch(trigger models.Trigger, defaultBranch string) bool {
	candidates := []string{defaultBranch}
	if defaultBranch == "" {
		candidates = defaultBranches
		named := slices.DeleteFunc(slices.Clone(candidates), func(branch string) bool {
			return !slices.ContainsFunc(append(slices.Clone(trigger.Branches), trigger.IgnoredBranches...), func(p string) bool {
				return strings.TrimPrefix(p, "!") == branch
			})
		})
		if len(named) > 0 {
			candidates = named
		}
	}
	for _, branch := range candidates {
		if (len(trigger.Branches) == 0 || branchFilter(trigger.Branches, branch)) &&
			!branchFilter(trigger.IgnoredBranches, branch) {
			return true
		}
	}
	return false
}

// branchFilter applies GitHub's branch patterns in order: * stops at a
// slash, ** does not, and a pattern starting with ! excludes what the ones
// before it included.
func branchFilter(patterns []string, branch string) bool {
	matched := false
	for _, pattern := range patterns {
		negated := strings.HasPrefix(pattern, "!")
		glob := strings.TrimPrefix(pattern, "!")
		expr := strings.NewReplacer(".", `\.`, "**", ".*", "*", "[^/]*").Replace(glob)
		if re, err := regexp.Compile("^" + expr + "$"); err == nil && re.MatchString(branch) {
			matched = !negated
		}
	}
	return matched
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
	checked, unclear := false, false
	for _, job := range jobs {
		if job.Check == "" {
			continue
		}
		if blocks(job, finding.Artifact.Jobs, required, map[string]bool{}) {
			return a
		}
		if job.UnnamedCheck() {
			unclear = true
		} else {
			checked = true
		}
	}
	switch {
	case unclear:
		a.Verdict = Unknown
	case checked:
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
