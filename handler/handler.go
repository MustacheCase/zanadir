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
	NewOutputService  output.Output
}

func Execute() error {
	return nil
}

func NewHandler(dir string) *Handler {
	return nil
}
