package rules

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"

	"github.com/MustacheCase/zanadir/models"
	"gopkg.in/yaml.v3"
)

type Collection struct {
	ByCategory map[string][]*Rule
	ByID       map[string]*Rule
	Skip       map[string]bool
}

type fileRule struct {
	ID         string   `yaml:"id"`
	ApplyOn    []string `yaml:"applyOn"`
	Categories []string `yaml:"categories"`
	Regex      string   `yaml:"regex"`
}

type Rule struct {
	ID         string
	ApplyOn    []string
	Categories []string
	Regex      *regexp.Regexp
	IsChecked  bool
}

type Service interface {
	GetCategoryRules(category models.Category) []*Rule
}

type service struct {
	RulesCollection *Collection
}

func (s *service) GetCategoryRules(category models.Category) []*Rule {
	return s.RulesCollection.ByCategory[string(category)]
}

func readYAMLRules() ([]fileRule, error) {
	var rules []fileRule

	err := filepath.WalkDir("./storage", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(path) != ".yaml" {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		var fileRules []fileRule
		if err := yaml.Unmarshal(data, &fileRules); err != nil {
			return fmt.Errorf("error parsing YAML file %s: %w", path, err)
		}

		rules = append(rules, fileRules...)
		return nil
	})

	if err != nil {
		return nil, err
	}
	return rules, nil
}

func convertRules(rules []fileRule) []*Rule {
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

func createRulesCollection() (*Collection, error) {
	rules, err := readYAMLRules()
	if err != nil {
		return nil, err
	}

	convertedRules := convertRules(rules)
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

func NewRulesService() (Service, error) {
	collection, err := createRulesCollection()
	if err != nil {
		return nil, err
	}

	return &service{
		RulesCollection: collection,
	}, nil
}
