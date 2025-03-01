package models

const (
	SCA       CategoryTitle = "SCA"
	Secrets   CategoryTitle = "Secrets"
	Licenses  CategoryTitle = "Licenses"
	EndOfLife CategoryTitle = "End Of Life"
	Table     Format        = "table"
	JSON      Format        = "json"
)

var CategoryTitles = []CategoryTitle{SCA, Secrets, Licenses, EndOfLife}

type CategoryTitle string

type Format string
