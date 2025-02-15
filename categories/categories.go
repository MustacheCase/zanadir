package categories

type Category struct {
	Name        string
	Description string
	Suggestions []*Suggestion
}

type Suggestion struct {
	Name        string
	Repository  string
	Description string
}
