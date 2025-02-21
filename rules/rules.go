package rules

import (
	"regexp"

	"github.com/MustacheCase/zanadir/models"
	"github.com/MustacheCase/zanadir/storage"
)

type Collection struct {
	ByCategory map[string][]*Rule
	ByID       map[string]*Rule
	Skip       map[string]bool
}

type Rule struct {
	ID         string
	ApplyOn    []string
	Categories []string
	Regex      *regexp.Regexp
	IsChecked  bool
}

type RuleService interface {
	GetCategoryRules(category models.CategoryTitle) []*Rule
}

type service struct {
	storageService  storage.Storage
	RulesCollection *Collection
}

func (s *service) GetCategoryRules(category models.CategoryTitle) []*Rule {
	return s.RulesCollection.ByCategory[string(category)]
}

func (s *service) convertRules(rules []storage.FileRule) []*Rule {
	var convertedRules []*Rule
	for _, r := range rules {
		convertedRules = append(convertedRules, &Rule{
			ID:         r.ID,
			ApplyOn:    r.ApplyOn,
			Categories: r.Categories,
			Regex:      regexp.MustCompile(r.Regex),
			IsChecked:  false,
		})
	}
	return convertedRules
}

func (s *service) createRulesCollection() (*Collection, error) {
	rules, err := s.storageService.ReadRules()
	if err != nil {
		return nil, err
	}

	convertedRules := s.convertRules(rules)
	categoryMap := make(map[string][]*Rule)
	idMap := make(map[string]*Rule)

	for _, r := range convertedRules {
		idMap[r.ID] = r
		for _, category := range r.Categories {
			categoryMap[category] = append(categoryMap[category], r)
		}
	}

	return &Collection{
		ByCategory: categoryMap,
		ByID:       idMap,
	}, nil
}

func NewRulesService(storageService storage.Storage) (RuleService, error) {
	s := &service{
		storageService: storageService,
	}
	collection, err := s.createRulesCollection()
	if err != nil {
		return nil, err
	}
	s.RulesCollection = collection

	return s, nil
}
