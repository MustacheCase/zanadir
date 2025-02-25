package models

const (
	SCA      CategoryTitle = "SCA"
	Secrets  CategoryTitle = "Secrets"
	Licenses CategoryTitle = "Licenses"
	Table    Format        = "table"
	JSON     Format        = "json"
)

var CategoryTitles = []CategoryTitle{SCA, Secrets, Licenses}

type CategoryTitle string

type Format string
