package models

const (
	SCA       CategoryTitle = "SCA"
	Secrets   CategoryTitle = "Secrets"
	Licenses  CategoryTitle = "Licenses"
	EndOfLife CategoryTitle = "End Of Life"
	Coverage  CategoryTitle = "Coverage"
	Linter    CategoryTitle = "Linter"
	Table     Format        = "table"
	JSON      Format        = "json"
)

var CategoryTitles = []CategoryTitle{SCA, Secrets, Licenses, EndOfLife, Coverage, Linter}

type CategoryTitle string

type Format string
