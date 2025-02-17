package matcher

type Match struct {
	Category string
	RuleID   string
	Location string
}

type Matcher interface {
}

type service struct {
}

func NewMatchService() Matcher {
	return service{}
}
