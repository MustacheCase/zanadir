package grading

import (
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
			got := ByCategory(tt.findings)["Secrets Detection"]
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
	verdicts := ByCategory([]*matcher.Finding{nil, nil})
	if len(verdicts) != 0 {
		t.Errorf("expected no categories, got %d", len(verdicts))
	}
}

func TestByCategorySeparatesCategories(t *testing.T) {
	pullRequest := []models.Trigger{{Event: "pull_request"}}

	sca := finding(pullRequest, &models.Job{})
	sca.Category = "SCA"
	secrets := finding(pullRequest, &models.Job{ContinueOnError: true})

	verdicts := ByCategory([]*matcher.Finding{sca, secrets})

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
