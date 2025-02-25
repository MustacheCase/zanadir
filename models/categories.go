package models

const (
	SCA     CategoryTitle = "SCA"
	Secrets CategoryTitle = "Secrets"
	License CategoryTitle = "License Compliance"
	Table   Format        = "table"
	JSON    Format        = "json"
)

var CategoryTitles = []CategoryTitle{SCA, Secrets, License}

type CategoryTitle string

type Format string
