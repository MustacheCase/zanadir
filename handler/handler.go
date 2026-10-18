package handler

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/MustacheCase/zanadir/applicability"
	"github.com/MustacheCase/zanadir/baseline"
	"github.com/MustacheCase/zanadir/config"
	"github.com/MustacheCase/zanadir/fixer"
	"github.com/MustacheCase/zanadir/gate"
	"github.com/MustacheCase/zanadir/grading"
	"github.com/MustacheCase/zanadir/language"
	"github.com/MustacheCase/zanadir/logger"
	"github.com/MustacheCase/zanadir/matcher"
	"github.com/MustacheCase/zanadir/models"
	"github.com/MustacheCase/zanadir/output"
	"github.com/MustacheCase/zanadir/parser"
	"github.com/MustacheCase/zanadir/remote"
	"github.com/MustacheCase/zanadir/rules"
	"github.com/MustacheCase/zanadir/scanner"
	"github.com/MustacheCase/zanadir/score"
	"github.com/MustacheCase/zanadir/suggester"
)

// debugFunc logs a step of the scan when --debug is on and does nothing
// otherwise.
type debugFunc func(format string, v ...interface{})

func newDebug(cfg *config.Config) debugFunc {
	if cfg.Debug {
		return logger.GetLogger().Info
	}
	return func(string, ...interface{}) {}
}

// stderr is where warnings go, keeping them out of a JSON or SARIF report.
var stderr io.Writer = os.Stderr

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
func weakened(excluded []string, dir string, findings []*matcher.Finding, required grading.Required, debugf debugFunc) []output.Weakness {
	skip := make(map[string]bool, len(excluded))
	for _, category := range excluded {
		skip[category] = true
	}

	assessments := grading.ByCategory(findings, required)
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
			Reason:   assessment.Reason,
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
	gate          *gate.Gate
	// skipped is every category left out of the score: excluded by the
	// operator or not applicable to the repository.
	skipped []string
}

// analyse runs the scan pipeline: it reads the CI configuration, matches it
// against the rules, and grades what it finds.
func (h *Handler) analyse(cfg *config.Config, debugf debugFunc, ignore func(string) bool) (*scanResult, error) {
	artifacts, err := h.ScanService.Scan(cfg.Dir)
	if err != nil {
		debugf("Scan error: %v", err)
		return nil, err
	}
	debugf("Found %d artifacts", len(artifacts))
	for _, a := range artifacts {
		if a != nil && a.Skipped != "" {
			_, _ = fmt.Fprintf(stderr, "Warning: skipped %s, which is not valid YAML (%s); the CI platform cannot run it either\n",
				repoRelative(cfg.Dir, a.Location), a.Skipped)
		}
	}

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
	skipped := append(append([]string{}, cfg.ExcludedCategories...), notApplicable...)

	suggestions := h.SuggestionService.FindSuggestions(findings, skipped, languages)
	debugf("Total suggestions: %d", len(suggestions))

	merge := mergeGate(cfg, artifacts)
	var required grading.Required
	if merge != nil {
		required = merge.Blocks
	}
	weaknesses := weakened(skipped, cfg.Dir, findings, required, debugf)
	debugf("Weakened categories: %d", len(weaknesses))

	return &scanResult{
		suggestions:   suggestions,
		artifacts:     artifacts,
		weaknesses:    weaknesses,
		notApplicable: notApplicable,
		gate:          merge,
		skipped:       skipped,
	}, nil
}

// readGate is gate.Read, replaced in tests.
var readGate = gate.Read

// mergeGate reads what a merge into the default branch waits for when asked
// to. A scan goes on without it rather than fail on a network error.
func mergeGate(cfg *config.Config, artifacts []*models.Artifact) *gate.Gate {
	if !cfg.RequiredChecks {
		return nil
	}
	repo, err := gate.Origin(cfg.Dir)
	if cfg.Repo != nil {
		repo, err = *cfg.Repo, nil
	}
	var merge *gate.Gate
	if err == nil {
		merge, err = readGate(repo, remote.Token())
	}
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "Warning: --required-checks could not read the merge settings (%v); grading without them\n", err)
		return nil
	}
	var checks []string
	for _, a := range artifacts {
		for _, job := range a.Jobs {
			if job.Check != "" {
				checks = append(checks, job.Check)
			}
		}
	}
	merge.Unproduced(checks)
	return merge
}

// Fix prints ready-to-paste CI configuration for the uncovered categories.
func (h *Handler) Fix(cfg *config.Config, w io.Writer) error {
	debugf := newDebug(cfg)
	result, err := h.analyse(cfg, debugf, fixer.IsGeneratedWorkflow)
	if err != nil {
		return err
	}
	suggestions := result.suggestions

	fixService, err := fixer.NewFixService()
	if err != nil {
		return err
	}

	platform := parser.Detect(cfg.Dir)
	debugf("Detected platform: %s", platform.ID)

	snippets := fixService.Snippets(suggestions, platform.ID)

	if !cfg.Write {
		return fixer.Render(w, snippets, platform)
	}

	if platform.ID != parser.GitHub {
		return fmt.Errorf("--write generates a GitHub Actions workflow; %s is not supported yet, run without --write to print the snippets", platform.ID)
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

// Execute scans the repository, reports the result, and fails when
// enforcement asks it to.
func (h *Handler) Execute(cfg *config.Config) error {
	debugf := newDebug(cfg)
	debugf("Starting scan for directory: %s", cfg.Dir)
	result, err := h.analyse(cfg, debugf, nil)
	if err != nil {
		return err
	}
	suggestions := result.suggestions

	coverage := score.Of(result.skipped, len(suggestions))
	debugf("Coverage score: %s", coverage)

	err = h.OutputService.Response(output.Report{
		Suggestions:   suggestions,
		Format:        cfg.Output,
		DestPath:      cfg.OutputFile,
		Anchor:        sarifAnchor(cfg.Dir, result.artifacts),
		Score:         coverage,
		Weakened:      result.weaknesses,
		NotApplicable: result.notApplicable,
		Platforms:     parser.Found(cfg.Dir),
		Gate:          reportGate(result.gate),
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
		acceptedWeaknesses := make(map[string]string, len(result.weaknesses))
		for _, weakness := range result.weaknesses {
			acceptedWeaknesses[weakness.Category] = weakness.Verdict
		}
		if err := baseline.Write(cfg.Baseline, uncovered, acceptedWeaknesses); err != nil {
			return err
		}
		debugf("Wrote %d categories to baseline %s", len(uncovered), cfg.Baseline)
		return nil
	}

	failing, err := h.enforceableCategories(cfg, uncovered, result.weaknesses, debugf)
	if err != nil {
		return err
	}

	if !failing.empty() {
		debugf("Failing scan: %s", failing.Error())
		return models.NewEnforceError(failing.Error())
	}

	debugf("Scan completed successfully")
	return nil
}

// enforcement is what should fail the scan. The two are kept apart because a
// missing category and a control that cannot fail a build are different
// problems and read as nonsense under one heading.
type enforcement struct {
	uncovered []string
	weakened  []string
}

func (e enforcement) empty() bool {
	return len(e.uncovered) == 0 && len(e.weakened) == 0
}

func (e enforcement) Error() string {
	var parts []string
	if len(e.uncovered) > 0 {
		parts = append(parts, "uncovered categories: "+strings.Join(e.uncovered, ", "))
	}
	if len(e.weakened) > 0 {
		parts = append(parts, "controls that cannot fail a build: "+strings.Join(e.weakened, ", "))
	}
	return strings.Join(parts, "; ")
}

// enforceableCategories narrows uncovered categories to those that should fail
// the scan: not accepted in the baseline, and named by --fail-on if given.
func (h *Handler) enforceableCategories(cfg *config.Config, uncovered []string, weaknesses []output.Weakness, debugf debugFunc) (enforcement, error) {
	var result enforcement
	if !cfg.Enforce && len(cfg.FailOn) == 0 && !cfg.FailOnWeakened {
		return result, nil
	}

	var accepted *baseline.Baseline
	if cfg.Baseline != "" {
		loaded, err := baseline.Load(cfg.Baseline)
		if err != nil {
			return result, err
		}
		accepted = loaded
		debugf("Loaded baseline %s with %d accepted categories", cfg.Baseline, len(loaded.Categories))
	}

	// FailOn is canonicalised by config, so an exact match is enough here.
	selected := make(map[string]bool, len(cfg.FailOn))
	for _, c := range cfg.FailOn {
		selected[c] = true
	}

	// An uncovered category only fails when enforcement was asked for;
	// --fail-on-weakened on its own is about weakened controls.
	if cfg.Enforce || len(cfg.FailOn) > 0 {
		for _, category := range uncovered {
			if accepted.Contains(category) {
				debugf("Category %s is accepted by the baseline", category)
				continue
			}
			if len(selected) > 0 && !selected[category] {
				continue
			}
			result.uncovered = append(result.uncovered, category)
		}
	}

	if !cfg.FailOnWeakened {
		return result, nil
	}

	for _, weakness := range weaknesses {
		if len(selected) > 0 && !selected[weakness.Category] {
			continue
		}
		if recorded, ok := accepted.AcceptedVerdict(weakness.Category); ok {
			if !grading.Worse(grading.Verdict(weakness.Verdict), grading.Verdict(recorded)) {
				debugf("Category %s is accepted by the baseline at %s", weakness.Category, recorded)
				continue
			}
			debugf("Category %s was accepted at %s but is now %s", weakness.Category, recorded, weakness.Verdict)
		}
		result.weakened = append(result.weakened, fmt.Sprintf("%s (%s)", weakness.Category, weakness.Verdict))
	}

	return result, nil
}

func Setup(custom ...rules.FileRule) (*Handler, error) {
	rulesService, err := rules.NewRulesService(custom...)
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

func reportGate(merge *gate.Gate) *output.Gate {
	if merge == nil {
		return nil
	}
	required := make([]string, 0, len(merge.Checks))
	for _, check := range merge.Checks {
		if !slices.Contains(required, check.Context) {
			required = append(required, check.Context)
		}
	}
	return &output.Gate{Branch: merge.Branch, Required: required, Scanning: merge.Scanning, Problems: merge.Problems}
}
