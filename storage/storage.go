package storage

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const suggestionsFile = "suggestions.yaml"

type FileRule struct {
	ID         string   `yaml:"id"`
	ApplyOn    []string `yaml:"applyOn"`
	Categories []string `yaml:"categories"`
	Regex      string   `yaml:"regex"`
}

type FileRules struct {
	Rules []FileRule `yaml:"rules"`
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

// CategoryFile file
type CategoryFile struct {
	Categories []CategorySuggestion `yaml:"categories"`
}

type Storage interface {
	ReadRules() ([]FileRule, error)
	ReadCategoriesSuggestions() ([]CategorySuggestion, error)
}

func (s *service) ReadRules() ([]FileRule, error) {
	var rules []FileRule

	basePath, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	absPath := filepath.Join(basePath, "rules", "storage")

	err = filepath.WalkDir(absPath, func(path string, d fs.DirEntry, err error) error {
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

		var fr FileRules
		if err := yaml.Unmarshal(data, &fr); err != nil {
			return fmt.Errorf("error parsing YAML file %s: %w", path, err)
		}

		rules = append(rules, fr.Rules...)
		return nil
	})

	if err != nil {
		return nil, err
	}
	return rules, nil
}

func (s *service) ReadCategoriesSuggestions() ([]CategorySuggestion, error) {
	basePath, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	absPath := filepath.Join(basePath, "suggester", suggestionsFile)

	data, err := os.ReadFile(absPath)
	if err != nil {
		return nil, err
	}

	var suggestions CategoryFile
	err = yaml.Unmarshal(data, &suggestions)
	if err != nil {
		return nil, err
	}

	return suggestions.Categories, nil
}

type service struct{}

func NewStorageService() Storage {
	return &service{}
}
