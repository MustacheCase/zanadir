package models

import "testing"

func TestHasTrigger(t *testing.T) {
	artifact := &Artifact{Triggers: []Trigger{
		{Event: "push", Branches: []string{"main"}},
		{Event: "schedule"},
	}}

	for event, want := range map[string]bool{
		"push":         true,
		"schedule":     true,
		"pull_request": false,
		"":             false,
	} {
		if got := artifact.HasTrigger(event); got != want {
			t.Errorf("HasTrigger(%q) = %v, want %v", event, got, want)
		}
	}
}

// An artifact from a parser that captures no triggers must not look like one
// that runs on nothing; callers have to tell the two apart themselves.
func TestHasTriggerWithNoneCaptured(t *testing.T) {
	if (&Artifact{}).HasTrigger("push") {
		t.Error("an artifact with no triggers should not report one")
	}
}

// RuleRun drops the followed commands, whole, and keeps what surrounds them.
func TestRuleRunDropsFollowedCommands(t *testing.T) {
	for _, tc := range []struct {
		name     string
		run      string
		followed []FollowedCommand
		want     string
	}{
		{"no followed commands", "npm run lint", nil, "npm run lint"},
		{"followed command is the whole run", "npm run lint", []FollowedCommand{{Command: "npm run lint"}}, ""},
		{"followed command before the expansion", "npm run lint\neslint .", []FollowedCommand{{Command: "npm run lint"}}, "\neslint ."},
		{"boundaries on either side are kept", "a && npm run lint || true", []FollowedCommand{{Command: "npm run lint"}}, "a &&  || true"},
		{"a longer command is not the followed one", "npm run lint:fix", []FollowedCommand{{Command: "npm run lint"}}, "npm run lint:fix"},
		{"a followed command not in the run is ignored", "make", []FollowedCommand{{Command: "npm run lint"}}, "make"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := (&Job{Run: tc.run, Followed: tc.followed}).RuleRunFor(nil); got != tc.want {
				t.Errorf("RuleRun() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRuleRunKeepsWrapperWhenExpansionDoesNotMatch(t *testing.T) {
	job := &Job{Run: "npm test\nnode scripts/runner.js", Followed: []FollowedCommand{{
		Command: "npm test", Expansion: []string{"node scripts/runner.js"},
	}}}
	if got := job.RuleRunFor(func(line string) bool { return line == "eslint ." }); got != job.Run {
		t.Errorf("RuleRunFor() = %q, want original run %q", got, job.Run)
	}
	if got := job.RuleRunFor(func(line string) bool { return line == "node scripts/runner.js" }); got != "\nnode scripts/runner.js" {
		t.Errorf("RuleRunFor() = %q, want expansion only", got)
	}
}

func TestJobCheckHelpers(t *testing.T) {
	for check, want := range map[string]bool{"${{ matrix.tool }}": true, " ${{ a }} ${{ b }} ": true, "Analyze (${{ matrix.language }})": false, "lint": false, "": false} {
		if got := (&Job{Check: check}).UnnamedCheck(); got != want {
			t.Errorf("UnnamedCheck(%q) = %v, want %v", check, got, want)
		}
	}
	for pkg, want := range map[string]bool{"./.github/workflows/scan.yml": true, "org/sec/.github/workflows/gitleaks.yml": true, "actions/checkout": false} {
		if got := (&Job{Package: pkg}).CallsWorkflow(); got != want {
			t.Errorf("CallsWorkflow(%q) = %v, want %v", pkg, got, want)
		}
	}
}
