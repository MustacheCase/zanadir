package handler

import (
	"github.com/MustacheCase/zanadir/matcher"
	"github.com/MustacheCase/zanadir/output"
	"github.com/MustacheCase/zanadir/rules"
	"github.com/MustacheCase/zanadir/scanner"
	"github.com/MustacheCase/zanadir/suggester"
)

type Handler struct {
	RulesService      rules.RuleService
	ScanService       scanner.Scanner
	SuggestionService suggester.Suggester
	MatchService      matcher.Matcher
	OutputService     output.Output
}

func (h *Handler) Execute() error {
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
