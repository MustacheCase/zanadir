package handler

import (
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"

	"github.com/MustacheCase/zanadir/applicability"
	"github.com/MustacheCase/zanadir/baseline"
	"github.com/MustacheCase/zanadir/config"
	"github.com/MustacheCase/zanadir/fixer"
	"github.com/MustacheCase/zanadir/grading"
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

// repoRelative turns a scanned path into one a report can point at. A path
// outside the scan directory yields nothing: worse than no location.
func repoRelative(dir, path string) string {
	if path == "" {
		return ""
	}
	rel, err := filepath.Rel(dir, path)
	if err != nil || strings.HasPrefix(rel, "..") {
		return ""
	}
	return filepath.ToSlash(rel)
}

// sarifAnchor returns a repo-relative CI file for SARIF results to point at
// when a finding has no location of its own.
func sarifAnchor(dir string, artifacts []*models.Artifact) string {
	for _, a := range artifacts {
		if a == nil {
			continue
		}
		if rel := repoRelative(dir, a.Location); rel != "" {
			return rel
		}
	}
	return ""
}

// weakened lists the covered categories whose tooling cannot fully protect the
// repository. An excluded category is left out: the operator has said it does
// not apply, and grading it anyway would argue with them.
func weakened(excluded []string, dir string, findings []*matcher.Finding, debugf func(string, ...interface{})) []output.Weakness {
	skip := make(map[string]bool, len(excluded))
	for _, category := range excluded {
		skip[category] = true
	}

	assessments := grading.ByCategory(findings)
	list := make([]output.Weakness, 0, len(assessments))
	for category, assessment := range assessments {
		debugf("Category %s is %s", category, assessment.Verdict)
		if skip[category] || !grading.Weakened(assessment.Verdict) {
			continue
		}
		list = append(list, output.Weakness{
			Category: category,
			Verdict:  string(assessment.Verdict),
			Location: repoRelative(dir, assessment.Location),
		})
	}

	// Map iteration order is random and a report has to be stable.
	sort.Slice(list, func(i, j int) bool { return list[i].Category < list[j].Category })
	return list
}

// scanResult is one run of the pipeline.
type scanResult struct {
	suggestions   []*suggester.CategorySuggestion
	artifacts     []*models.Artifact
	weaknesses    []output.Weakness
	notApplicable []string
}

// uncovered runs the scan pipeline and returns the categories with no tooling.
func (h *Handler) uncovered(cfg *config.Config, debugf func(string, ...interface{}), ignore func(string) bool) (*scanResult, error) {
	artifacts, err := h.ScanService.Scan(cfg.Dir)
	if err != nil {
		debugf("Scan error: %v", err)
		return nil, err
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

	notApplicable := applicability.NotApplicable(cfg.Dir)
	debugf("Not applicable: %v", notApplicable)

	// A category with nothing to scan is skipped exactly as an operator-
	// excluded one is, and leaves the score's denominator the same way.
	skip := append(append([]string{}, cfg.ExcludedCategories...), notApplicable...)

	suggestions := h.SuggestionService.FindSuggestions(findings, skip, languages)
	debugf("Total suggestions: %d", len(suggestions))

	weaknesses := weakened(skip, cfg.Dir, findings, debugf)
	debugf("Weakened categories: %d", len(weaknesses))

	return &scanResult{
		suggestions:   suggestions,
		artifacts:     artifacts,
		weaknesses:    weaknesses,
		notApplicable: notApplicable,
	}, nil
}

// Fix prints ready-to-paste CI configuration for the uncovered categories.
func (h *Handler) Fix(cfg *config.Config, w io.Writer) error {
	debugf := func(format string, v ...interface{}) {}
	if cfg.Debug {
		debugf = logger.GetLogger().Info
	}

	result, err := h.uncovered(cfg, debugf, fixer.IsGeneratedWorkflow)
	if err != nil {
		return err
	}
	suggestions := result.suggestions

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
	result, err := h.uncovered(cfg, debugf, nil)
	if err != nil {
		return err
	}
	suggestions := result.suggestions

	coverage := score.Of(append(append([]string{}, cfg.ExcludedCategories...), result.notApplicable...), len(suggestions))
	debugf("Coverage score: %s", coverage)

	err = h.OutputService.Response(output.Report{
		Suggestions:   suggestions,
		Format:        cfg.Output,
		DestPath:      cfg.OutputFile,
		Anchor:        sarifAnchor(cfg.Dir, result.artifacts),
		Score:         coverage,
		Weakened:      result.weaknesses,
		NotApplicable: result.notApplicable,
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
