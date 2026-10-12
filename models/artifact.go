package models

type Artifact struct {
	Name string
	// Empty means none were captured, not that it runs on everything.
	Triggers []Trigger
	Jobs     []*Job
	Location string
	// Declared in configuration rather than a CI job, such as Dependabot.
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
	// ContinueOnError covers GitLab's allow_failure too.
	ContinueOnError bool
	// Manual runs only when somebody asks.
	Manual bool
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
