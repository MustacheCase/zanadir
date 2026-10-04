package models

type Artifact struct {
	Name string
	// Triggers is what makes the workflow run. Empty means the parser found
	// none, not that the workflow runs on everything.
	Triggers []Trigger
	Jobs     []*Job
	Location string
}

// Trigger is one event a workflow runs on.
type Trigger struct {
	Event string
	// Branches is the branch filter; empty means every branch.
	Branches []string
}

type Job struct {
	Name    string
	Package string
	Version string
	// Run holds the shell command text of a step that invokes a tool directly.
	Run string
	// ContinueOnError is set when the step or its job sets it: either one means
	// a failure here cannot fail the build.
	ContinueOnError bool
	// If is the step's condition and JobIf the enclosing job's. Both gate the
	// step, and they are separate expressions, so they are not merged.
	If    string
	JobIf string
	Needs []string
	With  map[string]string
}

// HasTrigger reports whether the workflow runs on an event.
func (a *Artifact) HasTrigger(event string) bool {
	for _, t := range a.Triggers {
		if t.Event == event {
			return true
		}
	}
	return false
}
