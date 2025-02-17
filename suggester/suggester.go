package suggester

import (
	"os"

	"gopkg.in/yaml.v3"
)

const suggestionsFile = "suggestions.yaml"

type service struct{}

// Category represents a category of suggestions
type Category struct {
	ID          string        `yaml:"id"`
	Name        string        `yaml:"name"`
	Description string        `yaml:"description"`
	Suggestions []*Suggestion `yaml:"suggestions"`
}

// Suggestion represents a single suggestion
type Suggestion struct {
	Name        string `yaml:"name"`
	Repository  string `yaml:"repository"`
	Description string `yaml:"description"`
	Language    string `yaml:"language"`
}

type Suggester interface {
	GetSuggestions() ([]Category, error)
}

func (s *service) GetSuggestions() ([]Category, error) {
	data, err := os.ReadFile(suggestionsFile)
	if err != nil {
		return nil, err
	}

	var suggestions []Category
	err = yaml.Unmarshal(data, &suggestions)
	if err != nil {
		return nil, err
	}

	return suggestions, nil
}

func NewSuggestionService() Suggester {
	return &service{}
}
