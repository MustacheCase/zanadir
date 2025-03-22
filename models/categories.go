package models

const (
	SCA       CategoryTitle = "SCA"
	Secrets   CategoryTitle = "Secrets"
	Licenses  CategoryTitle = "Licenses"
	EndOfLife CategoryTitle = "End Of Life"
	Coverage  CategoryTitle = "Coverage"
	Linter    CategoryTitle = "Linter"
	PerformanceTesting    CategoryTitle = "Performance Testing"
	Table     Format        = "table"
	JSON      Format        = "json"
)

var CategoryTitles = []CategoryTitle{SCA, Secrets, Licenses, EndOfLife, Coverage, Linter, PerformanceTesting}

type CategoryTitle string

type Format string
