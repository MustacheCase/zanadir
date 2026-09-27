package handler

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/MustacheCase/zanadir/baseline"
	"github.com/MustacheCase/zanadir/config"
	"github.com/MustacheCase/zanadir/fixer"
	"github.com/MustacheCase/zanadir/language"
	"github.com/MustacheCase/zanadir/logger"
	"github.com/MustacheCase/zanadir/matcher"
	"github.com/MustacheCase/zanadir/models"
	"github.com/MustacheCase/zanadir/output"
	"github.com/MustacheCase/zanadir/rules"
	"github.com/MustacheCase/zanadir/scanner"
	"github.com/MustacheCase/zanadir/score"
	"github.com/MustacheCase/zanadir/suggester"
)

type Handler struct {
	RulesService      rules.RuleService
	ScanService       scanner.Scanner
	MatchService      matcher.Matcher
	SuggestionService suggester.Suggester
	OutputService     output.Output
}

// sarifAnchor returns a repo-relative CI file for SARIF results to point at.
// A path outside the scan directory is skipped: worse than no location.
func sarifAnchor(dir string, artifacts []*models.Artifact) string {
	for _, a := range artifacts {
		if a == nil || a.Location == "" {
			continue
		}
		rel, err := filepath.Rel(dir, a.Location)
		if err != nil || strings.HasPrefix(rel, "..") {
			continue
		}
		return filepath.ToSlash(rel)
	}
	return ""
}

// uncovered runs the scan pipeline and returns the categories with no tooling.
func (h *Handler) uncovered(cfg *config.Config, debugf func(string, ...interface{}), ignore func(string) bool) ([]*suggester.CategorySuggestion, []*models.Artifact, error) {
	artifacts, err := h.ScanService.Scan(cfg.Dir)
	if err != nil {
		debugf("Scan error: %v", err)
		return nil, nil, err
	}
	debugf("Found %d artifacts", len(artifacts))

	if ignore != nil {
		kept := artifacts[:0]
		for _, a := range artifacts {
			if a != nil && ignore(a.Location) {
				debugf("Ignoring %s", a.Location)
				continue
			}
			kept = append(kept, a)
		}
		artifacts = kept
	}

	var findings []*matcher.Finding
	for _, c := range models.CategoryTitles {
		categoryRules := h.RulesService.GetCategoryRules(c)
		findings = append(findings, h.MatchService.Match(artifacts, categoryRules)...)
	}
	debugf("Total findings: %d", len(findings))

	languages := language.Detect(cfg.Dir)
	debugf("Detected languages: %v", languages)

	suggestions := h.SuggestionService.FindSuggestions(findings, cfg.ExcludedCategories, languages)
	debugf("Total suggestions: %d", len(suggestions))

	return suggestions, artifacts, nil
}

// Fix prints ready-to-paste CI configuration for the uncovered categories.
func (h *Handler) Fix(cfg *config.Config, w io.Writer) error {
	debugf := func(format string, v ...interface{}) {}
	if cfg.Debug {
		debugf = logger.GetLogger().Info
	}

	suggestions, _, err := h.uncovered(cfg, debugf, fixer.IsGeneratedWorkflow)
	if err != nil {
		return err
	}

	fixService, err := fixer.NewFixService()
	if err != nil {
		return err
	}

	platform := fixer.DetectPlatform(cfg.Dir)
	debugf("Detected platform: %s", platform)

	snippets := fixService.Snippets(suggestions, platform)

	if !cfg.Write {
		return fixer.Render(w, snippets, platform)
	}

	if platform != fixer.PlatformGitHub {
		return fmt.Errorf("--write generates a GitHub Actions workflow; %s is not supported yet, run without --write to print the snippets", platform)
	}
	if len(snippets) == 0 {
		_, err := fmt.Fprintln(w, "Nothing to write: every uncovered category either has no template yet, or there is nothing uncovered.")
		return err
	}

	path, err := fixer.WriteWorkflow(cfg.Dir, snippets)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "Wrote %s (%d categories).\nIt runs as its own job, so it repeats checkout and adds a check to pull requests.\n", path, len(snippets))
	return err
}

func (h *Handler) Execute(cfg *config.Config) error {
	debugf := func(format string, v ...interface{}) {}
	if cfg.Debug {
		debugf = logger.GetLogger().Info
	}

	debugf("Starting scan for directory: %s", cfg.Dir)
	suggestions, artifacts, err := h.uncovered(cfg, debugf, nil)
	if err != nil {
		return err
	}

	coverage := score.Of(cfg.ExcludedCategories, len(suggestions))
	debugf("Coverage score: %s", coverage)

	err = h.OutputService.Response(output.Report{
		Suggestions: suggestions,
		Format:      cfg.Output,
		DestPath:    cfg.OutputFile,
		Anchor:      sarifAnchor(cfg.Dir, artifacts),
		Score:       coverage,
	})
	if err != nil {
		debugf("Output error: %v", err)
		return err
	}

	if cfg.Badge != "" {
		if err := score.Write(cfg.Badge, coverage); err != nil {
			return err
		}
		debugf("Wrote badge %s", cfg.Badge)
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
