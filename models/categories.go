package suggester

const (
	SCA     CategoryType = "SCA"
	Secrets CategoryType = "Secrets"
	Table   Format       = "table"
	JSON    Format       = "json"
)

type CategoryType string

type Format string
