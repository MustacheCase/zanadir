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
