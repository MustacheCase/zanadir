package models

type Artifact struct {
	Name string
	// Triggers empty means none were captured, not that the workflow runs on
	// everything.
	Triggers []Trigger
	Jobs     []*Job
	Location string
	// OutsidePipeline marks a control declared in configuration rather than a
	// CI job, such as Dependabot: no triggers because there is no pipeline.
	OutsidePipeline bool
}

type Trigger struct {
	Event    string
	Branches []string
}

type Job struct {
	Name    string
	Package string
	Version string
	// Run is the shell command of a step that invokes a tool directly.
	Run string
	// ContinueOnError is set when the step or its job sets it.
	ContinueOnError bool
	// If is the step's condition, JobIf the enclosing job's.
	If    string
	JobIf string
	Needs []string
	With  map[string]string
}

func (a *Artifact) HasTrigger(event string) bool {
	for _, t := range a.Triggers {
		if t.Event == event {
			return true
		}
	}
	return false
}
