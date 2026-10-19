package grading

import (
	"cmp"
	"regexp"
	"testing"

	"github.com/MustacheCase/zanadir/matcher"
	"github.com/MustacheCase/zanadir/models"
	"github.com/MustacheCase/zanadir/rules"
)

func finding(triggers []models.Trigger, job *models.Job) *matcher.Finding {
	return &matcher.Finding{
		Category: "Secrets Detection",
		Artifact: &models.Artifact{Name: "ci", Triggers: triggers, Location: "ci.yml"},
		Job:      job,
	}
}

func TestGrade(t *testing.T) {
	pullRequest := []models.Trigger{{Event: "pull_request"}}
	pushOnly := []models.Trigger{{Event: "push", Branches: []string{"main"}}}
	scheduled := []models.Trigger{{Event: "schedule"}, {Event: "workflow_dispatch"}}

	tests := []struct {
		name     string
		finding  *matcher.Finding
		expected Verdict
	}{
		{
			name:     "gating a pull request can fail the build",
			finding:  finding(pullRequest, &models.Job{Package: "gitleaks/gitleaks-action"}),
			expected: Enforcing,
		},
		{
			name:     "merge_group gates too",
			finding:  finding([]models.Trigger{{Event: "merge_group"}}, &models.Job{}),
			expected: Enforcing,
		},
		{
			name:     "pushes only never gate a pull request",
			finding:  finding(pushOnly, &models.Job{}),
			expected: Partial,
		},
		{
			name:     "a schedule blocks nothing",
			finding:  finding(scheduled, &models.Job{}),
			expected: Advisory,
		},
		{
			// The signal the whole feature exists for.
			name:     "continue-on-error cannot fail the build however it is triggered",
			finding:  finding(pullRequest, &models.Job{ContinueOnError: true}),
			expected: Advisory,
		},
		{
			name:     "a step condition cannot be read, so no claim is made",
			finding:  finding(pullRequest, &models.Job{If: "github.ref == 'refs/heads/main'"}),
			expected: Unknown,
		},
		{
			name:     "a job condition counts the same as a step one",
			finding:  finding(pullRequest, &models.Job{JobIf: "github.event_name == 'push'"}),
			expected: Unknown,
		},
		{
			// A parser that captures no triggers must not look like a workflow
			// that runs on nothing.
			name:     "no triggers captured is unknown, not unreachable",
			finding:  finding(nil, &models.Job{}),
			expected: Unknown,
		},
		{
			name:     "a workflow-name match has only triggers to go on",
			finding:  finding(pullRequest, nil),
			expected: Enforcing,
		},
		{
			name:     "continue-on-error outranks an unreadable condition",
			finding:  finding(pullRequest, &models.Job{ContinueOnError: true, If: "always()"}),
			expected: Advisory,
		},
		{
			name:     "a finding with no artifact says nothing",
			finding:  &matcher.Finding{Category: "SCA"},
			expected: Unknown,
		},
		{
			name:     "a nil finding says nothing",
			finding:  nil,
			expected: Unknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Grade(tt.finding); got != tt.expected {
				t.Errorf("Grade() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestWeakened(t *testing.T) {
	for verdict, want := range map[Verdict]bool{
		Advisory:  true,
		Partial:   true,
		Enforcing: false,
		// Not knowing is not a finding.
		Unknown: false,
	} {
		if got := Weakened(verdict); got != want {
			t.Errorf("Weakened(%q) = %v, want %v", verdict, got, want)
		}
	}
}

// One properly wired control is enough, and one unreadable control keeps the
// category quiet rather than reporting a weakness that may not exist.
func TestByCategoryKeepsTheStrongestVerdict(t *testing.T) {
	pullRequest := []models.Trigger{{Event: "pull_request"}}

	at := func(location string, f *matcher.Finding) *matcher.Finding {
		f.Location = location
		f.Artifact.Location = location
		return f
	}

	tests := []struct {
		name             string
		findings         []*matcher.Finding
		expectedVerdict  Verdict
		expectedLocation string
	}{
		{
			name: "an enforcing control outranks a weak one beside it",
			findings: []*matcher.Finding{
				at("weak.yml", finding(pullRequest, &models.Job{ContinueOnError: true})),
				at("good.yml", finding(pullRequest, &models.Job{})),
			},
			expectedVerdict:  Enforcing,
			expectedLocation: "good.yml",
		},
		{
			name: "order does not matter",
			findings: []*matcher.Finding{
				at("good.yml", finding(pullRequest, &models.Job{})),
				at("weak.yml", finding(pullRequest, &models.Job{ContinueOnError: true})),
			},
			expectedVerdict:  Enforcing,
			expectedLocation: "good.yml",
		},
		{
			name: "an unreadable control keeps the category quiet",
			findings: []*matcher.Finding{
				at("weak.yml", finding(pullRequest, &models.Job{ContinueOnError: true})),
				at("cond.yml", finding(pullRequest, &models.Job{If: "always()"})),
			},
			expectedVerdict:  Unknown,
			expectedLocation: "cond.yml",
		},
		{
			name: "partial beats advisory",
			findings: []*matcher.Finding{
				at("sched.yml", finding([]models.Trigger{{Event: "schedule"}}, &models.Job{})),
				at("push.yml", finding([]models.Trigger{{Event: "push"}}, &models.Job{})),
			},
			expectedVerdict:  Partial,
			expectedLocation: "push.yml",
		},
		{
			name: "all weak stays weak and points at the evidence",
			findings: []*matcher.Finding{
				at("weak.yml", finding(pullRequest, &models.Job{ContinueOnError: true})),
			},
			expectedVerdict:  Advisory,
			expectedLocation: "weak.yml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ByCategory(tt.findings, nil)["Secrets Detection"]
			if got.Verdict != tt.expectedVerdict {
				t.Errorf("verdict = %q, want %q", got.Verdict, tt.expectedVerdict)
			}
			if got.Location != tt.expectedLocation {
				t.Errorf("location = %q, want %q", got.Location, tt.expectedLocation)
			}
		})
	}
}

func TestByCategorySkipsNilFindings(t *testing.T) {
	verdicts := ByCategory([]*matcher.Finding{nil, nil}, nil)
	if len(verdicts) != 0 {
		t.Errorf("expected no categories, got %d", len(verdicts))
	}
}

func TestByCategorySeparatesCategories(t *testing.T) {
	pullRequest := []models.Trigger{{Event: "pull_request"}}

	sca := finding(pullRequest, &models.Job{})
	sca.Category = "SCA"
	secrets := finding(pullRequest, &models.Job{ContinueOnError: true})

	verdicts := ByCategory([]*matcher.Finding{sca, secrets}, nil)

	if verdicts["SCA"].Verdict != Enforcing {
		t.Errorf("SCA = %q, want enforcing", verdicts["SCA"].Verdict)
	}
	if verdicts["Secrets Detection"].Verdict != Advisory {
		t.Errorf("Secrets Detection = %q, want advisory", verdicts["Secrets Detection"].Verdict)
	}
}

// The verdict list in the rules package is duplicated there because it cannot
// import this one without a cycle. If the two drift, a rule file could assert
// a verdict grading does not recognise and the mismatch would be silent.
func TestRuleVerdictsAreAllKnownHere(t *testing.T) {
	known := map[Verdict]bool{Enforcing: true, Partial: true, Advisory: true, Unknown: true}

	for _, verdict := range rules.WeakenedVerdicts {
		if !known[Verdict(verdict)] {
			t.Errorf("rules allows verdict %q, which grading does not know", verdict)
		}
		if !Weakened(Verdict(verdict)) {
			t.Errorf("rules allows verdict %q, which grading would not report", verdict)
		}
	}
}

// Generic signals cannot see a flag that defeats one particular tool.
func TestGradeToolSpecificFlags(t *testing.T) {
	pullRequest := []models.Trigger{{Event: "pull_request"}}

	exitCodeZero := &rules.Rule{
		ID:       "trivy-rule",
		Weakened: []rules.WeakenedWhen{{RunMatches: regexp.MustCompile(`--exit-code[= ]+0\b`), Verdict: "advisory"}},
	}
	gitleaks := &rules.Rule{
		ID:       "gitleaks-rule",
		Regex:    regexp.MustCompile(`\bgitleaks\b`),
		Weakened: []rules.WeakenedWhen{{RunMatches: regexp.MustCompile(`--exit-code[= ]+0\b`), Verdict: "advisory"}},
	}
	checkov := &rules.Rule{
		ID:       "checkov-rule",
		Regex:    regexp.MustCompile(`\bcheckov\b`),
		Weakened: []rules.WeakenedWhen{{RunMatches: regexp.MustCompile(`--soft-fail(?:\s|$)`), Verdict: "advisory"}},
	}
	softFail := &rules.Rule{
		ID:       "tfsec-rule",
		Weakened: []rules.WeakenedWhen{{Input: "soft_fail", Equals: "true", Verdict: "advisory"}},
	}

	tests := []struct {
		name     string
		rule     *rules.Rule
		job      *models.Job
		expected Verdict
	}{
		{
			name:     "a flag that stops the tool failing",
			rule:     exitCodeZero,
			job:      &models.Job{Run: "trivy fs --exit-code 0 ."},
			expected: Advisory,
		},
		{
			name:     "the same flag written with an equals sign",
			rule:     exitCodeZero,
			job:      &models.Job{Run: "trivy fs --exit-code=0 ."},
			expected: Advisory,
		},
		{
			// The false positive that would matter most: a correctly wired
			// tool must not be called weak.
			name:     "a non-zero exit code is the tool working",
			rule:     exitCodeZero,
			job:      &models.Job{Run: "trivy fs --exit-code 1 ."},
			expected: Enforcing,
		},
		{
			name:     "a neighbouring tool's flag is not this tool's",
			rule:     gitleaks,
			job:      &models.Job{Run: "gitleaks detect\ntrivy fs --exit-code 0 ."},
			expected: Enforcing,
		},
		{
			name:     "nor in the same line, after &&",
			rule:     gitleaks,
			job:      &models.Job{Run: "trivy fs --exit-code 0 . && gitleaks detect"},
			expected: Enforcing,
		},
		{
			name:     "the tool's own flag still weakens it",
			rule:     gitleaks,
			job:      &models.Job{Run: "trivy fs .\ngitleaks detect --exit-code 0"},
			expected: Advisory,
		},
		{
			name:     "soft-fail-on softens only the checks it lists",
			rule:     checkov,
			job:      &models.Job{Run: "checkov -d . --soft-fail-on CKV_AWS_20"},
			expected: Enforcing,
		},
		{
			name:     "soft-fail softens them all",
			rule:     checkov,
			job:      &models.Job{Run: "checkov -d . --soft-fail"},
			expected: Advisory,
		},
		{
			name:     "a step matched by its action is read whole",
			rule:     checkov,
			job:      &models.Job{Package: "bridgecrewio/checkov-action", Run: "--soft-fail"},
			expected: Advisory,
		},
		{
			name:     "an action input that disables failure",
			rule:     softFail,
			job:      &models.Job{Package: "aquasecurity/tfsec-action", With: map[string]string{"soft_fail": "true"}},
			expected: Advisory,
		},
		{
			name:     "the input compared case-insensitively",
			rule:     softFail,
			job:      &models.Job{With: map[string]string{"soft_fail": "TRUE"}},
			expected: Advisory,
		},
		{
			name:     "an input set at run time may be either",
			rule:     softFail,
			job:      &models.Job{With: map[string]string{"soft_fail": "${{ inputs.soft }}"}},
			expected: Unknown,
		},
		{
			name:     "or by an Azure variable",
			rule:     softFail,
			job:      &models.Job{With: map[string]string{"soft_fail": "$(softFail)"}},
			expected: Unknown,
		},
		{
			name:     "the input set to the safe value",
			rule:     softFail,
			job:      &models.Job{With: map[string]string{"soft_fail": "false"}},
			expected: Enforcing,
		},
		{
			name:     "the input absent entirely",
			rule:     softFail,
			job:      &models.Job{With: map[string]string{"additional_args": "--x"}},
			expected: Enforcing,
		},
		{
			name:     "a rule with no checks",
			rule:     &rules.Rule{ID: "plain"},
			job:      &models.Job{Run: "semgrep ci"},
			expected: Enforcing,
		},
		{
			name:     "no rule at all",
			rule:     nil,
			job:      &models.Job{Run: "semgrep ci"},
			expected: Enforcing,
		},
		{
			// An explicit flag is a statement of fact; an if: is a guess.
			name:     "a tool-specific flag outranks a condition that cannot be read",
			rule:     exitCodeZero,
			job:      &models.Job{Run: "trivy fs --exit-code 0 .", If: "always()"},
			expected: Advisory,
		},
		{
			name:     "continue-on-error still wins",
			rule:     exitCodeZero,
			job:      &models.Job{Run: "trivy fs --exit-code 1 .", ContinueOnError: true},
			expected: Advisory,
		},
		{
			// A run-matching check must not fire on a step that has no command.
			name:     "a package step is not matched by a run check",
			rule:     exitCodeZero,
			job:      &models.Job{Package: "aquasecurity/trivy-action"},
			expected: Enforcing,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := finding(pullRequest, tt.job)
			f.Rule = tt.rule
			if got := Grade(f); got != tt.expected {
				t.Errorf("Grade() = %q, want %q", got, tt.expected)
			}
		})
	}
}

// Dependabot and Renovate run on the forge's schedule and open pull requests.
// They never stand between a change and the branch, which is advisory - and
// specifically not unknown, because there is nothing unreadable about them.
func TestGradeControlsDeclaredOutsideThePipeline(t *testing.T) {
	bot := &matcher.Finding{
		Category: "SCA",
		Artifact: &models.Artifact{
			Name:            "dependabot",
			Location:        ".github/dependabot.yml",
			OutsidePipeline: true,
		},
		Job: &models.Job{Name: "dependabot", Package: "dependabot"},
	}

	if got := Grade(bot); got != Advisory {
		t.Errorf("Grade() = %q, want advisory", got)
	}

	// It outranks the trigger analysis that would otherwise call it unknown.
	if got := gradeTriggers(bot.Artifact); got != Unknown {
		t.Errorf("without the marker the triggers alone give %q, so the marker is what makes this advisory", got)
	}
}

// A job that only runs when somebody asks cannot gate anything, the same way
// GitHub's workflow_dispatch cannot.
func TestGradeManualJob(t *testing.T) {
	pullRequest := []models.Trigger{{Event: "pull_request"}}

	if got := Grade(finding(pullRequest, &models.Job{Manual: true})); got != Advisory {
		t.Errorf("Grade() = %q, want advisory", got)
	}
	// It outranks a condition that cannot be read.
	if got := Grade(finding(pullRequest, &models.Job{Manual: true, If: "always()"})); got != Advisory {
		t.Errorf("Grade() = %q, want advisory", got)
	}
}

func TestWorse(t *testing.T) {
	for _, tc := range []struct {
		a, b Verdict
		want bool
	}{
		{Advisory, Partial, true},
		{Advisory, Enforcing, true},
		{Partial, Enforcing, true},
		{Partial, Advisory, false},
		{Enforcing, Advisory, false},
		{Advisory, Advisory, false},
		// Unknown sits above the reported verdicts, so a control going from
		// unreadable to advisory counts as getting worse.
		{Advisory, Unknown, true},
		{Unknown, Advisory, false},
	} {
		if got := Worse(tc.a, tc.b); got != tc.want {
			t.Errorf("Worse(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}

// Supply chain tools are judged on the path that ships, not the one that
// gates, so an SBOM made at release time is not a weakness.
func TestGradeSupplyChainOnTheShippingPath(t *testing.T) {
	grade := func(category string, triggers []models.Trigger, job *models.Job) Verdict {
		return Grade(&matcher.Finding{
			Category: category,
			Artifact: &models.Artifact{Triggers: triggers, Location: "sbom.yml"},
			Job:      job,
		})
	}
	release := []models.Trigger{{Event: "release"}}

	tests := []struct {
		name     string
		category string
		triggers []models.Trigger
		job      *models.Job
		expected Verdict
	}{
		{"on release", "Supply Chain", release, &models.Job{Run: "syft ."}, Enforcing},
		{"on a tag push", "Supply Chain", []models.Trigger{{Event: "push", Branches: []string{"main"}}}, &models.Job{}, Enforcing},
		{"on a Bitbucket tag", "Supply Chain", []models.Trigger{{Event: "tag"}}, &models.Job{}, Enforcing},
		{"after another workflow", "Supply Chain", []models.Trigger{{Event: "workflow_run"}}, &models.Job{}, Enforcing},
		{"on pull requests", "Supply Chain", []models.Trigger{{Event: "pull_request"}}, &models.Job{}, Enforcing},
		{"only on a schedule", "Supply Chain", []models.Trigger{{Event: "schedule"}}, &models.Job{}, Advisory},
		{"by hand, which may be the release", "Supply Chain", []models.Trigger{{Event: "workflow_dispatch"}}, &models.Job{}, Unknown},
		{"by hand or on a schedule", "Supply Chain", []models.Trigger{{Event: "schedule"}, {Event: "workflow_dispatch"}}, &models.Job{}, Unknown},
		{"a manual step in a release pipeline", "Supply Chain", release, &models.Job{Manual: true}, Unknown},
		{"a manual scanner is still advisory", "SCA", release, &models.Job{Manual: true}, Advisory},
		{"no triggers captured", "Supply Chain", nil, &models.Job{}, Unknown},
		{"signing that may fail silently", "Supply Chain", release, &models.Job{ContinueOnError: true}, Advisory},
		{"a scanner on release is still advisory", "SCA", release, &models.Job{}, Advisory},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := grade(tt.category, tt.triggers, tt.job); got != tt.expected {
				t.Errorf("Grade() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestByCategoryHoldsPullRequestControlsToRequiredChecks(t *testing.T) {
	pr := []models.Trigger{{Event: "pull_request"}}
	push := []models.Trigger{{Event: "push"}}
	required := func(job *models.Job) bool { return job.Check == "Static analysis" }
	job := func(check string) *models.Job { return &models.Job{Check: check} }
	byName := func(jobs ...*models.Job) *matcher.Finding {
		f := finding(pr, nil)
		f.Artifact.Jobs = jobs
		return f
	}
	gateJob := func(triggers []models.Trigger, twoLevels bool) *matcher.Finding {
		unit := &models.Job{Name: "unit-linux", Check: "Unit tests (Linux)"}
		f := finding(triggers, unit)
		gate := &models.Job{Name: "gate", Check: "Static analysis", Needs: []string{"unit-linux"}}
		f.Artifact.Jobs = []*models.Job{unit, {Name: "other", Check: "other", Needs: []string{"unit-linux"}}, gate}
		if twoLevels {
			middle := &models.Job{Name: "middle", Check: "middle", Needs: []string{"unit-linux"}}
			gate.Needs = []string{"middle"}
			f.Artifact.Jobs = []*models.Job{unit, middle, middle, gate}
		}
		return f
	}
	shipping := finding([]models.Trigger{{Event: "release"}}, job("release"))
	shipping.Category = string(models.SupplyChain)

	tests := []struct {
		name     string
		finding  *matcher.Finding
		required Required
		want     Verdict
		reason   string
	}{
		{"a required check enforces", finding(pr, job("Static analysis")), required, Enforcing, ""},
		{"a check that is not required is partial", finding(pr, job("lint")), required, Partial, NotRequired},
		{"without the merge settings nothing changes", finding(pr, job("lint")), nil, Enforcing, ""},
		{"a job of another platform has no check", finding(pr, &models.Job{}), required, Enforcing, ""},
		{"only an enforcing verdict is held back", finding(push, job("lint")), required, Partial, ""},
		{"supply chain is judged on the path that ships", shipping, required, Enforcing, ""},
		{"a workflow name match counts any of its jobs", byName(&models.Job{}, job("lint"), job("Static analysis")), required, Enforcing, ""},
		{"a workflow name match with no required job", byName(job("lint")), required, Partial, NotRequired},
		{"a workflow name match with no jobs", byName(), required, Enforcing, ""},
		{"a job named only by an expression cannot be matched", finding(pr, job("${{ matrix.tool }}")), required, Unknown, ""},
		{"an unmatched name outranks a known one", byName(job("${{ matrix.tool }}"), job("lint")), required, Unknown, ""},
		{"a required gate job holds its needs to it", gateJob(pr, false), required, Enforcing, ""},
		{"through more than one level", gateJob(pr, true), required, Enforcing, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ByCategory([]*matcher.Finding{tt.finding}, tt.required)[tt.finding.Category]
			if got.Verdict != tt.want || got.Reason != tt.reason {
				t.Errorf("got %q (%q), want %q (%q)", got.Verdict, got.Reason, tt.want, tt.reason)
			}
		})
	}
}

// A name match cannot say which step runs the tool, so it is graded from the
// triggers - unless no step of the workflow can fail.
func TestGradeOfAWorkflowNameMatch(t *testing.T) {
	pr := []models.Trigger{{Event: "pull_request"}}
	named := func(jobs ...*models.Job) *matcher.Finding {
		f := finding(pr, nil)
		f.Artifact.Jobs = jobs
		return f
	}
	tests := []struct {
		name    string
		finding *matcher.Finding
		want    Verdict
	}{
		{"every step may fail", named(&models.Job{ContinueOnError: true}, &models.Job{ContinueOnError: true}), Advisory},
		{"one step can fail", named(&models.Job{ContinueOnError: true}, &models.Job{}), Enforcing},
		{"no steps captured", named(), Enforcing},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Grade(tt.finding); got != tt.want {
				t.Errorf("Grade = %q, want %q", got, tt.want)
			}
		})
	}
}

// A pull request trigger gates only if it runs on the changes themselves and
// on pull requests to the default branch.
func TestPullRequestFilters(t *testing.T) {
	pr := func(trigger models.Trigger) []models.Trigger {
		trigger.Event = cmp.Or(trigger.Event, "pull_request")
		return []models.Trigger{trigger}
	}
	tests := []struct {
		name     string
		triggers []models.Trigger
		want     Verdict
	}{
		{"no filters", pr(models.Trigger{}), Enforcing},
		{"review types", pr(models.Trigger{Types: []string{"opened", "labeled"}}), Enforcing},
		{"closed only runs after the merge", pr(models.Trigger{Types: []string{"closed"}}), Partial},
		{"pull_request_target closed only", pr(models.Trigger{Event: "pull_request_target", Types: []string{"closed", "labeled"}}), Partial},
		{"merge_group types are its own", pr(models.Trigger{Event: "merge_group", Types: []string{"checks_requested"}}), Enforcing},
		{"to main", pr(models.Trigger{Branches: []string{"main"}}), Enforcing},
		{"to trunk", pr(models.Trigger{Branches: []string{"trunk"}}), Enforcing},
		{"a glob admitting main", pr(models.Trigger{Branches: []string{"ma*"}}), Enforcing},
		{"** admits anything", pr(models.Trigger{Branches: []string{"**"}}), Enforcing},
		{"* stops at a slash", pr(models.Trigger{Branches: []string{"release/*"}}), Unknown},
		{"another branch only", pr(models.Trigger{Branches: []string{"legacy-v1"}}), Unknown},
		{"negated after including", pr(models.Trigger{Branches: []string{"**", "!main"}}), Unknown},
		{"ignoring main", pr(models.Trigger{IgnoredBranches: []string{"main"}}), Unknown},
		{"ignoring something else", pr(models.Trigger{IgnoredBranches: []string{"docs/**"}}), Enforcing},
		{"a bad pattern matches nothing", pr(models.Trigger{Branches: []string{"[main"}}), Unknown},
		{"one gating trigger wins", []models.Trigger{{Event: "pull_request", Types: []string{"closed"}}, {Event: "pull_request", Branches: []string{"main"}}}, Enforcing},
		{"unclear outranks push", []models.Trigger{{Event: "push"}, {Event: "pull_request", Branches: []string{"legacy-v1"}}}, Unknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Grade(finding(tt.triggers, &models.Job{})); got != tt.want {
				t.Errorf("Grade = %q, want %q", got, tt.want)
			}
		})
	}
}

// Where the default branch was read, it is the only candidate, which settles
// what the stand-in names could only guess.
func TestGradeWithTheRealDefaultBranch(t *testing.T) {
	for _, tt := range []struct {
		name, branch string
		trigger      models.Trigger
		want         Verdict
	}{
		{"develop, but the filter names master", "develop", models.Trigger{Branches: []string{"master"}}, Unknown},
		{"an unusual default the filter names", "stable", models.Trigger{Branches: []string{"stable"}}, Enforcing},
		{"a glob admitting it", "release/2.x", models.Trigger{Branches: []string{"release/**"}}, Enforcing},
		{"ignoring it", "stable", models.Trigger{IgnoredBranches: []string{"stable"}}, Unknown},
		{"no filter", "stable", models.Trigger{}, Enforcing},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tt.trigger.Event = "pull_request"
			f := finding([]models.Trigger{tt.trigger}, &models.Job{})
			f.Artifact.DefaultBranch = tt.branch
			if got := Grade(f); got != tt.want {
				t.Errorf("Grade = %q, want %q", got, tt.want)
			}
		})
	}
}
