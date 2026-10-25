// Package suggester proposes tools for the categories a scan found uncovered.
package suggester

import (
	"embed"
	"fmt"
	"slices"
	"strings"

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
	// Platform is the one CI system a tool reads, such as "github" or
	// "gitlab"; empty when it reads any.
	Platform string `yaml:"platform"`
}

// CategoryFile holds all category suggestions.
type CategoryFile struct {
	Categories []CategorySuggestion `yaml:"categories"`
}

//go:embed suggestions.yaml
var suggestionsFS embed.FS

// Suggester picks tools for the categories a scan found uncovered.
type Suggester interface {
	FindSuggestions(findings []*matcher.Finding, excludedCategories []string, languages []string, platforms []string) []*CategorySuggestion
}

// filterByLanguage drops tools targeting a language the repository does not
// use. Tools with no language are agnostic and always kept. If filtering would
// empty a category, the unfiltered set is returned instead.
func filterByLanguage(suggestions []*Suggestion, languages []string) []*Suggestion {
	if len(languages) == 0 || len(suggestions) == 0 {
		return suggestions
	}

	detected := make(map[string]bool, len(languages))
	for _, l := range languages {
		detected[strings.ToLower(l)] = true
	}

	filtered := make([]*Suggestion, 0, len(suggestions))
	for _, sug := range suggestions {
		if sug.Language == "" || detected[strings.ToLower(sug.Language)] {
			filtered = append(filtered, sug)
		}
	}

	if len(filtered) == 0 {
		return suggestions
	}
	return filtered
}

// filterByPlatform keeps the tools that read a CI system the repository uses,
// so a GitLab project is not pointed at a GitHub Actions linter. Like the
// language filter, it keeps everything when nothing would be left.
func filterByPlatform(suggestions []*Suggestion, platforms []string) []*Suggestion {
	if len(platforms) == 0 || len(suggestions) == 0 {
		return suggestions
	}
	filtered := make([]*Suggestion, 0, len(suggestions))
	for _, sug := range suggestions {
		if sug.Platform == "" || slices.Contains(platforms, sug.Platform) {
			filtered = append(filtered, sug)
		}
	}
	if len(filtered) == 0 {
		return suggestions
	}
	return filtered
}

func (s *service) FindSuggestions(findings []*matcher.Finding, excludedCategories []string, languages []string, platforms []string) []*CategorySuggestion {
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
				// Copy before filtering: CategoriesMap is the shared catalogue.
				categoriesSuggestions = append(categoriesSuggestions, &CategorySuggestion{
					ID:          category.ID,
					Name:        category.Name,
					Description: category.Description,
					Suggestions: filterByPlatform(filterByLanguage(category.Suggestions, languages), platforms),
				})
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
			Platform:    sug.Platform,
		})
	}
	return result
}

// validateCategoriesMap ensures every category title resolves to a suggestions entry.
func validateCategoriesMap(categoriesMap map[string]*CategorySuggestion) error {
	var missing []string
	for _, title := range models.CategoryTitles {
		if _, ok := categoriesMap[string(title)]; !ok {
			missing = append(missing, string(title))
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("suggestions.yaml is missing an entry for category %s", strings.Join(missing, ", "))
	}
	return nil
}

// newService builds a validated service from a catalogue, so the validation can
// be exercised with a catalogue the embedded one can never produce.
func newService(cats []CategorySuggestion) (*service, error) {
	s := &service{CategoriesMap: buildCategoriesMap(cats)}
	if err := validateCategoriesMap(s.CategoriesMap); err != nil {
		return nil, err
	}
	return s, nil
}

// NewSuggestionService returns a Suggester backed by the embedded catalogue.
func NewSuggestionService() (Suggester, error) {
	cats, err := readEmbeddedSuggestions()
	if err != nil {
		return nil, err
	}
	return newService(cats)
}

// Catalogue returns the embedded suggestions, for callers that extend it.
func Catalogue() ([]CategorySuggestion, error) {
	return readEmbeddedSuggestions()
}
