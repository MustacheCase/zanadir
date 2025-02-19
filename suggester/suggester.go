package suggester

import (
	"os"

	"github.com/MustacheCase/zanadir/matcher"
	"github.com/MustacheCase/zanadir/models"
	"gopkg.in/yaml.v3"
)

const suggestionsFile = "suggestions.yaml"

type service struct{}

// CategorySuggestion represents a category of suggestions
type CategorySuggestion struct {
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
	CategorySuggestion() ([]CategorySuggestion, error)
	FindSuggestions(findings []*matcher.Finding) ([]*CategorySuggestion, error)
}

func (s *service) CategorySuggestion() ([]CategorySuggestion, error) {
	data, err := os.ReadFile(suggestionsFile)
	if err != nil {
		return nil, err
	}

	var suggestions []CategorySuggestion
	err = yaml.Unmarshal(data, &suggestions)
	if err != nil {
		return nil, err
	}

	return suggestions, nil
}

func (s *service) FindSuggestions(findings []*matcher.Finding) ([]*CategorySuggestion, error) {
	var categoriesSuggestions []*CategorySuggestion
	coveredCategories := make(map[string]bool)
	for _, f := range findings {
		coveredCategories[f.Category] = true
	}
	categoriesMap, err := s.CategorySuggestionMap()
	if err != nil {
		return nil, err
	}
	// check which of the known categories is not covered
	for _, title := range models.CategoryTitles {
		if exists := coveredCategories[string(title)]; !exists {
			if category, ok := categoriesMap[string(title)]; ok {
				categoriesSuggestions = append(categoriesSuggestions, category)
			}
		}
	}

	return categoriesSuggestions, nil
}

func (s *service) CategorySuggestionMap() (map[string]*CategorySuggestion, error) {
	categoriesMap := make(map[string]*CategorySuggestion)
	categories, err := s.CategorySuggestion()
	if err != nil {
		return nil, err
	}
	for _, c := range categories {
		categoriesMap[c.ID] = &c
	}

	return categoriesMap, nil
}

func NewSuggestionService() Suggester {
	return &service{}
}
