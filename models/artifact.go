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
}
