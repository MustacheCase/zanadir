package models

const (
	SCA                CategoryTitle = "SCA"
	Secrets            CategoryTitle = "Secrets Detection"
	Licenses           CategoryTitle = "License Compliance"
	EndOfLife          CategoryTitle = "End Of Life"
	Coverage           CategoryTitle = "Coverage"
	Linter             CategoryTitle = "Linter"
	PerformanceTesting CategoryTitle = "Performance Testing"
	UnitTests          CategoryTitle = "Unit Tests"
	Table              Format        = "table"
	JSON               Format        = "json"
)

var CategoryTitles = []CategoryTitle{SCA, Secrets, Licenses, EndOfLife, Coverage, Linter, PerformanceTesting, UnitTests}

type CategoryTitle string

type Format string
