package output

import "github.com/MustacheCase/zanadir/suggester"

type Output interface {
	Response([]*suggester.CategorySuggestion) error
}

type service struct{}

func (s *service) Response([]*suggester.CategorySuggestion) error {
	return nil
}

func NewOutputService() Output {
	return &service{}
}
