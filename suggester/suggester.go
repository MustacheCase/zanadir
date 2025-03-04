package suggester

import (
	"github.com/MustacheCase/zanadir/matcher"
	"github.com/MustacheCase/zanadir/models"
	"github.com/MustacheCase/zanadir/storage"
)

type service struct {
	storageService storage.Storage
	CategoriesMap  map[string]*storage.CategorySuggestion
}

type Suggester interface {
	FindSuggestions(findings []*matcher.Finding, excludedCategories []string) []*storage.CategorySuggestion
}

func (s *service) FindSuggestions(findings []*matcher.Finding, excludedCategories []string) []*storage.CategorySuggestion {
	var categoriesSuggestions []*storage.CategorySuggestion
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

func (s *service) categorySuggestionMap() (map[string]*storage.CategorySuggestion, error) {
	categoriesMap := make(map[string]*storage.CategorySuggestion)
	categories, err := s.storageService.ReadCategoriesSuggestions()
	if err != nil {
		return nil, err
	}
	for _, c := range categories {
		categoriesMap[c.ID] = &c
	}

	return categoriesMap, nil
}

func NewSuggestionService(storageService storage.Storage) (Suggester, error) {
	s := &service{
		storageService: storageService,
	}

	categoriesMap, err := s.categorySuggestionMap()
	if err != nil {
		return nil, err
	}
	s.CategoriesMap = categoriesMap

	return s, nil
}
