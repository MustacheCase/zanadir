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
