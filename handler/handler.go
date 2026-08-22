package handler

import (
	"fmt"
	"strings"

	"github.com/MustacheCase/zanadir/baseline"
	"github.com/MustacheCase/zanadir/config"
	"github.com/MustacheCase/zanadir/language"
	"github.com/MustacheCase/zanadir/logger"
	"github.com/MustacheCase/zanadir/matcher"
	"github.com/MustacheCase/zanadir/models"
	"github.com/MustacheCase/zanadir/output"
	"github.com/MustacheCase/zanadir/rules"
	"github.com/MustacheCase/zanadir/scanner"
	"github.com/MustacheCase/zanadir/suggester"
)

type Handler struct {
	RulesService      rules.RuleService
	ScanService       scanner.Scanner
	MatchService      matcher.Matcher
	SuggestionService suggester.Suggester
	OutputService     output.Output
}

func (h *Handler) Execute(cfg *config.Config) error {
	debugf := func(format string, v ...interface{}) {}
	if cfg.Debug {
		debugf = logger.GetLogger().Info
	}

	debugf("Starting scan for directory: %s", cfg.Dir)
	artifacts, err := h.ScanService.Scan(cfg.Dir)
	if err != nil {
		debugf("Scan error: %v", err)
		return err
	}
	debugf("Found %d artifacts", len(artifacts))

	var findings []*matcher.Finding
	for _, c := range models.CategoryTitles {
		categoryRules := h.RulesService.GetCategoryRules(c)
		categoryFindings := h.MatchService.Match(artifacts, categoryRules)
		findings = append(findings, categoryFindings...)
	}
	debugf("Total findings: %d", len(findings))

	languages := language.Detect(cfg.Dir)
	debugf("Detected languages: %v", languages)

	suggestions := h.SuggestionService.FindSuggestions(findings, cfg.ExcludedCategories, languages)
	debugf("Total suggestions: %d", len(suggestions))

	err = h.OutputService.Response(suggestions, cfg.Output, cfg.OutputFile)
	if err != nil {
		debugf("Output error: %v", err)
		return err
	}

	uncovered := make([]string, 0, len(suggestions))
	for _, s := range suggestions {
		uncovered = append(uncovered, s.ID)
	}

	if cfg.WriteBaseline {
		if err := baseline.Write(cfg.Baseline, uncovered); err != nil {
			return err
		}
		debugf("Wrote %d categories to baseline %s", len(uncovered), cfg.Baseline)
		return nil
	}

	failing, err := h.enforceableCategories(cfg, uncovered, debugf)
	if err != nil {
		return err
	}

	if len(failing) > 0 {
		debugf("Failing scan for categories: %s", strings.Join(failing, ", "))
		return models.NewEnforceError(fmt.Sprintf("uncovered categories: %s", strings.Join(failing, ", ")))
	}

	debugf("Scan completed successfully")
	return nil
}

// enforceableCategories narrows uncovered categories to those that should fail
// the scan: not accepted in the baseline, and named by --fail-on if given.
func (h *Handler) enforceableCategories(cfg *config.Config, uncovered []string, debugf func(string, ...interface{})) ([]string, error) {
	if !cfg.Enforce && len(cfg.FailOn) == 0 {
		return nil, nil
	}

	var accepted *baseline.Baseline
	if cfg.Baseline != "" {
		loaded, err := baseline.Load(cfg.Baseline)
		if err != nil {
			return nil, err
		}
		accepted = loaded
		debugf("Loaded baseline %s with %d accepted categories", cfg.Baseline, len(loaded.Categories))
	}

	// FailOn is canonicalised by config, so an exact match is enough here.
	selected := make(map[string]bool, len(cfg.FailOn))
	for _, c := range cfg.FailOn {
		selected[c] = true
	}

	var failing []string
	for _, category := range uncovered {
		if accepted.Contains(category) {
			debugf("Category %s is accepted by the baseline", category)
			continue
		}
		if len(selected) > 0 && !selected[category] {
			continue
		}
		failing = append(failing, category)
	}
	return failing, nil
}

func Setup() (*Handler, error) {
	rulesService, err := rules.NewRulesService()
	if err != nil {
		return nil, err
	}
	repoScanner := scanner.NewRepositoryScanner()
	scanService := scanner.NewScanService(repoScanner)
	suggestionService, err := suggester.NewSuggestionService()
	if err != nil {
		return nil, err
	}
	matchService := matcher.NewMatchService()
	outputService := output.NewOutputService()

	return NewHandler(rulesService, scanService, suggestionService, matchService, outputService), nil
}

func NewHandler(rulesService rules.RuleService, scanService scanner.Scanner,
	suggestionService suggester.Suggester, matchService matcher.Matcher,
	outputService output.Output) *Handler {

	return &Handler{
		RulesService:      rulesService,
		ScanService:       scanService,
		SuggestionService: suggestionService,
		MatchService:      matchService,
		OutputService:     outputService,
	}
}
