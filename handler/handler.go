package handler

import (
	"github.com/MustacheCase/zanadir/config"
	"github.com/MustacheCase/zanadir/matcher"
	"github.com/MustacheCase/zanadir/models"
	"github.com/MustacheCase/zanadir/output"
	"github.com/MustacheCase/zanadir/parser"
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

func (h *Handler) Execute(config *config.Config) error {
	artifacts, err := h.ScanService.Scan(config.Dir)
	if err != nil {
		return err
	}

	var findings []*matcher.Finding
	for _, c := range models.CategoryTitles {
		categoryRules := h.RulesService.GetCategoryRules(c)
		categoryFindings := h.MatchService.Match(artifacts, categoryRules)
		findings = append(findings, categoryFindings...)
	}

	suggestions := h.SuggestionService.FindSuggestions(findings, config.ExcludedCategories)

	err = h.OutputService.Response(suggestions)
	if err != nil {
		return err
	}

	if config.Strict && len(suggestions) > 0 {
		return models.NewStrictError("Strict mode enabled and suggestions found")
	}

	return nil
}

func Setup() (*Handler, error) {
	storageService := storage.NewStorageService()
	rulesService, err := rules.NewRulesService(storageService)
	if err != nil {
		return nil, err
	}
	githubParser := parser.NewGithubParser()
	repoScanner := scanner.NewRepositoryScanner(githubParser)
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
