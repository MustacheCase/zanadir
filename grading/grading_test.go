package grading

import (
	"testing"

	"github.com/MustacheCase/zanadir/matcher"
	"github.com/MustacheCase/zanadir/models"
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
