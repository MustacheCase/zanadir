package handler

import (
	"github.com/MustacheCase/zanadir/config"
	"github.com/MustacheCase/zanadir/logger"
	"github.com/MustacheCase/zanadir/matcher"
	"github.com/MustacheCase/zanadir/models"
	"github.com/MustacheCase/zanadir/output"
	"github.com/MustacheCase/zanadir/rules"
	"github.com/MustacheCase/zanadir/scanner"
	"github.com/MustacheCase/zanadir/storage"
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

	suggestions := h.SuggestionService.FindSuggestions(findings, cfg.ExcludedCategories)
	debugf("Total suggestions: %d", len(suggestions))

	err = h.OutputService.Response(suggestions)
	if err != nil {
		debugf("Output error: %v", err)
		return err
	}

	if cfg.Enforce && len(suggestions) > 0 {
		debugf("Enforce mode enabled and suggestions found, failing scan")
		return models.NewEnforceError("Enforce mode enabled and suggestions found")
	}

	debugf("Scan completed successfully")
	return nil
}

func Setup() (*Handler, error) {
	storageService := storage.NewStorageService()
	rulesService, err := rules.NewRulesService(storageService)
	if err != nil {
		return nil, err
	}
	repoScanner := scanner.NewRepositoryScanner()
	scanService := scanner.NewScanService(repoScanner)
	suggestionService, err := suggester.NewSuggestionService(storageService)
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
