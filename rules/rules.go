package rules

import (
	"embed"
	"fmt"
	"regexp"
	"strings"

	"github.com/MustacheCase/zanadir/models"
	"gopkg.in/yaml.v3" // added YAML import
)

//go:embed storage/*
var rulesFS embed.FS

// applyOn field selectors understood by the matcher.
const (
	FieldArtifactName = "Artifact.Name"
	FieldJobName      = "Job.Name"
	FieldJobPackage   = "Job.Package"
	FieldJobRun       = "Job.Run"
)

// SupportedFields lists every selector matchesRule handles.
var SupportedFields = []string{FieldArtifactName, FieldJobName, FieldJobPackage, FieldJobRun}

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

// validateRules rejects unknown applyOn selectors and categories, which would
// otherwise leave the rule silently matching nothing.
func validateRules(fileRules []FileRule) error {
	supported := make(map[string]bool, len(SupportedFields))
	for _, f := range SupportedFields {
		supported[f] = true
	}
	knownCategory := make(map[string]bool, len(models.CategoryTitles))
	for _, c := range models.CategoryTitles {
		knownCategory[string(c)] = true
	}

	for _, r := range fileRules {
		for _, field := range r.ApplyOn {
			if !supported[field] {
				return fmt.Errorf("rule %q: unknown applyOn field %q, expected one of %s",
					r.ID, field, strings.Join(SupportedFields, ", "))
			}
		}
		for _, category := range r.Categories {
			if !knownCategory[category] {
				return fmt.Errorf("rule %q: unknown category %q", r.ID, category)
			}
		}
	}
	return nil
}

func (s *service) createRulesCollection() (*Collection, error) {
	rules, err := readEmbeddedRules()
	if err != nil {
		return nil, err
	}

	if err := validateRules(rules); err != nil {
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
