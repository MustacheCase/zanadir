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
	ID         string             `yaml:"id"`
	ApplyOn    []string           `yaml:"applyOn"`
	Categories []string           `yaml:"categories"`
	Regex      string             `yaml:"regex"`
	Weakened   []FileWeakenedWhen `yaml:"weakenedWhen"`
}

// FileWeakenedWhen is one way a tool can be configured so that it cannot
// protect anything. Deliberately only two shapes - a regex over the shell
// command, or a single action input compared to a value - so that this stays a
// narrow escape hatch for tool-specific flags rather than growing into an
// expression language inside YAML. Everything that weakens any control,
// continue-on-error and the triggers, is handled generically in Go.
type FileWeakenedWhen struct {
	RunMatches string `yaml:"runMatches"`
	Input      string `yaml:"input"`
	Equals     string `yaml:"equals"`
	Verdict    string `yaml:"verdict"`
}

// WeakenedVerdicts are the verdicts a rule may assert. The list is repeated
// here rather than taken from the grading package, which imports the matcher,
// which imports this one; a test in grading keeps the two in step.
var WeakenedVerdicts = []string{"advisory", "partial"}

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
	Weakened   []WeakenedWhen
}

// WeakenedWhen is a compiled FileWeakenedWhen.
type WeakenedWhen struct {
	RunMatches *regexp.Regexp
	Input      string
	Equals     string
	Verdict    string
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
		weakened := make([]WeakenedWhen, 0, len(r.Weakened))
		for _, w := range r.Weakened {
			check := WeakenedWhen{Input: w.Input, Equals: w.Equals, Verdict: w.Verdict}
			if w.RunMatches != "" {
				check.RunMatches = regexp.MustCompile(w.RunMatches)
			}
			weakened = append(weakened, check)
		}

		convertedRules = append(convertedRules, &Rule{
			ID:         r.ID,
			ApplyOn:    r.ApplyOn,
			Categories: r.Categories,
			Regex:      regexp.MustCompile(r.Regex),
			IsChecked:  false,
			Weakened:   weakened,
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
		if err := validateWeakened(r); err != nil {
			return err
		}
	}
	return nil
}

// validateWeakened rejects a check that would silently do nothing, and
// compiles its regex here so a typo is an error at startup rather than a panic
// from MustCompile.
func validateWeakened(r FileRule) error {
	allowed := make(map[string]bool, len(WeakenedVerdicts))
	for _, v := range WeakenedVerdicts {
		allowed[v] = true
	}

	for _, w := range r.Weakened {
		switch {
		case w.RunMatches == "" && w.Input == "":
			return fmt.Errorf("rule %q: a weakenedWhen needs runMatches or input", r.ID)
		case w.RunMatches != "" && w.Input != "":
			return fmt.Errorf("rule %q: a weakenedWhen takes runMatches or input, not both", r.ID)
		case w.Input != "" && w.Equals == "":
			return fmt.Errorf("rule %q: weakenedWhen input %q needs equals", r.ID, w.Input)
		case !allowed[w.Verdict]:
			return fmt.Errorf("rule %q: weakenedWhen verdict %q, expected one of %s",
				r.ID, w.Verdict, strings.Join(WeakenedVerdicts, ", "))
		}
		if w.RunMatches != "" {
			if _, err := regexp.Compile(w.RunMatches); err != nil {
				return fmt.Errorf("rule %q: weakenedWhen runMatches %q: %w", r.ID, w.RunMatches, err)
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
