package models

type Artifact struct {
	Name     string
	Jobs     []*Job
	Location string
}

type Job struct {
	Name    string
	Package string
	Version string
	// Run holds the shell command text of a step that invokes a tool directly.
	Run string
}
