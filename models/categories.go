package models

const (
	SCA     CategoryTitle = "SCA"
	Secrets CategoryTitle = "Secrets"
	Table   Format        = "table"
	JSON    Format        = "json"
)

var CategoryTitles = []CategoryTitle{SCA, Secrets}

type CategoryTitle string

type Format string
