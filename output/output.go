package output

import (
	"encoding/json"
	"fmt"

	"github.com/MustacheCase/zanadir/storage"
)

type Output interface {
	Response(suggestions []*storage.CategorySuggestion) error
}

type service struct{}

func (s *service) Response(suggestions []*storage.CategorySuggestion) error {
	data, err := json.MarshalIndent(suggestions, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal suggestions: %w", err)
	}

	// Print the formatted JSON to stdout
	fmt.Println(string(data))
	return nil
}

func NewOutputService() Output {
	return &service{}
}
