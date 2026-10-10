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

// One properly wired control is enough, and one unreadable control keeps the
// category quiet rather than reporting a weakness that may not exist.
func TestBest(t *testing.T) {
	tests := []struct {
		name     string
		verdicts []Verdict
		expected Verdict
	}{
		{"nothing to go on", nil, Unknown},
		{"one enforcing control is enough", []Verdict{Advisory, Enforcing, Partial}, Enforcing},
		{"unknown outranks the reportable verdicts", []Verdict{Advisory, Unknown}, Unknown},
		{"partial beats advisory", []Verdict{Advisory, Partial}, Partial},
		{"all advisory stays advisory", []Verdict{Advisory, Advisory}, Advisory},
		{"enforcing beats unknown", []Verdict{Unknown, Enforcing}, Enforcing},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Best(tt.verdicts); got != tt.expected {
				t.Errorf("Best(%v) = %q, want %q", tt.verdicts, got, tt.expected)
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

func TestByCategory(t *testing.T) {
	pullRequest := []models.Trigger{{Event: "pull_request"}}

	weak := finding(pullRequest, &models.Job{ContinueOnError: true})
	strong := finding(pullRequest, &models.Job{})
	strong.Category = "SCA"

	other := finding([]models.Trigger{{Event: "schedule"}}, &models.Job{})
	other.Category = "Secrets Detection"

	verdicts := ByCategory([]*matcher.Finding{weak, strong, other, nil})

	if verdicts["SCA"] != Enforcing {
		t.Errorf("SCA = %q, want enforcing", verdicts["SCA"])
	}
	// Both of its controls are advisory, so the category is.
	if verdicts["Secrets Detection"] != Advisory {
		t.Errorf("Secrets Detection = %q, want advisory", verdicts["Secrets Detection"])
	}
	if len(verdicts) != 2 {
		t.Errorf("expected 2 categories, got %d", len(verdicts))
	}
}
