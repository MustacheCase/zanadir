package handler

import (
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

func (h *Handler) Execute(dir string) error {
	artifacts, err := h.ScanService.Scan(dir)
	if err != nil {
		return err
	}

	var findings []*matcher.Finding
	for _, c := range models.CategoryTitles {
		categoryRules := h.RulesService.GetCategoryRules(c)
		categoryFindings := h.MatchService.Match(artifacts, categoryRules)
		findings = append(findings, categoryFindings...)
	}

	suggestions, err := h.SuggestionService.FindSuggestions(findings)
	if err != nil {
		return err
	}

	err = h.OutputService.Response(suggestions)
	if err != nil {
		return err
	}

	return nil
}

func NewHandler() (*Handler, error) {
	rulesService, err := rules.NewRulesService()
	if err != nil {
		return nil, err
	}
	return &Handler{
		RulesService:      rulesService,
		ScanService:       scanner.NewScanService(),
		SuggestionService: suggester.NewSuggestionService(),
		MatchService:      matcher.NewMatchService(),
		OutputService:     output.NewOutputService(),
	}, nil
}
