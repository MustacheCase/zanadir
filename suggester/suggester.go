package suggester

import (
	"embed"

	"github.com/MustacheCase/zanadir/matcher"
	"github.com/MustacheCase/zanadir/models"
	"gopkg.in/yaml.v3"
)

type service struct {
	CategoriesMap map[string]*CategorySuggestion
}

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

// CategoryFile holds all category suggestions.
type CategoryFile struct {
	Categories []CategorySuggestion `yaml:"categories"`
}

//go:embed suggestions.yaml
var suggestionsFS embed.FS

type Suggester interface {
	FindSuggestions(findings []*matcher.Finding, excludedCategories []string) []*CategorySuggestion
}

func (s *service) FindSuggestions(findings []*matcher.Finding, excludedCategories []string) []*CategorySuggestion {
	var categoriesSuggestions []*CategorySuggestion
	coveredCategories := make(map[string]bool)
	for _, f := range findings {
		coveredCategories[f.Category] = true
	}

	exclusionMap := make(map[string]struct{})
	for _, category := range excludedCategories {
		exclusionMap[category] = struct{}{}
	}

	// check which of the known categories is not covered
	for _, title := range models.CategoryTitles {
		if _, excluded := exclusionMap[string(title)]; excluded {
			continue
		}
		if exists := coveredCategories[string(title)]; !exists {
			if category, ok := s.CategoriesMap[string(title)]; ok {
				categoriesSuggestions = append(categoriesSuggestions, category)
			}
		}
	}

	return categoriesSuggestions
}

func readEmbeddedSuggestions() ([]CategorySuggestion, error) {
	// Read the embedded YAML file directly.
	data, err := suggestionsFS.ReadFile("suggestions.yaml")
	if err != nil {
		return nil, err
	}
	var suggestionFile CategoryFile
	if err := yaml.Unmarshal(data, &suggestionFile); err != nil {
		return nil, err
	}
	return suggestionFile.Categories, nil
}

// buildCategoriesMap converts embedded CategorySuggestion slice to a map of CategorySuggestion.
func buildCategoriesMap(cats []CategorySuggestion) map[string]*CategorySuggestion {
	categoriesMap := make(map[string]*CategorySuggestion)
	for _, cat := range cats {
		categoriesMap[cat.ID] = &CategorySuggestion{
			ID:          cat.ID,
			Name:        cat.Name,
			Description: cat.Description,
			Suggestions: convertSuggestions(cat.Suggestions),
		}
	}
	return categoriesMap
}

func convertSuggestions(sugs []*Suggestion) []*Suggestion {
	var result []*Suggestion
	for _, sug := range sugs {
		result = append(result, &Suggestion{
			Name:        sug.Name,
			Repository:  sug.Repository,
			Description: sug.Description,
			Language:    sug.Language,
		})
	}
	return result
}

func NewSuggestionService() (Suggester, error) {
	s := &service{}
	cats, err := readEmbeddedSuggestions()
	if err != nil {
		return nil, err
	}
	s.CategoriesMap = buildCategoriesMap(cats)
	return s, nil
}
