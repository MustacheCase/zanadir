package rules

import (
	"embed"
	"regexp"

	"github.com/MustacheCase/zanadir/models"
	"gopkg.in/yaml.v3" // added YAML import
)

//go:embed storage/*
var rulesFS embed.FS

type FileRule struct {
	ID         string   `yaml:"id"`
	ApplyOn    []string `yaml:"applyOn"`
	Categories []string `yaml:"categories"`
	Regex      string   `yaml:"regex"`
}

type FileRules struct {
	Rules []FileRule `yaml:"rules"`
}

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
	RulesCollection *Collection
}

func (s *service) GetCategoryRules(category models.CategoryTitle) []*Rule {
	return s.RulesCollection.ByCategory[string(category)]
}

func (s *service) convertRules(rules []FileRule) []*Rule {
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
	rules, err := readEmbeddedRules()
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

func readEmbeddedRules() ([]FileRule, error) {
	entries, err := rulesFS.ReadDir("storage")
	if err != nil {
		return nil, err
	}

	var rules []FileRule
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		data, err := rulesFS.ReadFile("storage/" + entry.Name())
		if err != nil {
			return nil, err
		}
		var fileRules FileRules
		if err := yaml.Unmarshal(data, &fileRules); err != nil {
			return nil, err
		}
		rules = append(rules, fileRules.Rules...)
	}
	return rules, nil
}

func NewRulesService() (RuleService, error) {
	s := &service{}
	collection, err := s.createRulesCollection()
	if err != nil {
		return nil, err
	}
	s.RulesCollection = collection

	return s, nil
}
