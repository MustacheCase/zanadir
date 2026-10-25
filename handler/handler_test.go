package handler

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/MustacheCase/zanadir/baseline"
	"github.com/MustacheCase/zanadir/config"
	"github.com/MustacheCase/zanadir/gate"
	"github.com/MustacheCase/zanadir/grading"
	"github.com/MustacheCase/zanadir/internal/testrepo"
	"github.com/MustacheCase/zanadir/matcher"
	"github.com/MustacheCase/zanadir/models"
	"github.com/MustacheCase/zanadir/output"
	"github.com/MustacheCase/zanadir/remote"
	"github.com/MustacheCase/zanadir/rules"
	"github.com/MustacheCase/zanadir/score"
	"github.com/MustacheCase/zanadir/suggester"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type MockRuleService struct{ mock.Mock }

type MockScanner struct{ mock.Mock }

type MockMatcher struct{ mock.Mock }

type MockSuggester struct{ mock.Mock }

type MockOutput struct{ mock.Mock }

func (m *MockRuleService) GetCategoryRules(category models.CategoryTitle) []*rules.Rule {
	args := m.Called(category)
	return args.Get(0).([]*rules.Rule)
}

func (m *MockScanner) Scan(dir string) ([]*models.Artifact, error) {
	args := m.Called(dir)
	return args.Get(0).([]*models.Artifact), args.Error(1)
}

func (m *MockMatcher) Match(artifacts []*models.Artifact, ruleSet []*rules.Rule) []*matcher.Finding {
	args := m.Called(artifacts, ruleSet)
	return args.Get(0).([]*matcher.Finding)
}

func (m *MockSuggester) FindSuggestions(findings []*matcher.Finding, excludedCategories []string, languages []string, platforms []string) []*suggester.CategorySuggestion {
	args := m.Called(findings)
	return args.Get(0).([]*suggester.CategorySuggestion)
}

func (m *MockOutput) Response(report output.Report) error {
	args := m.Called(report)
	return args.Error(0)
}

var (
	mockRuleService  *MockRuleService
	mockScanner      *MockScanner
	mockMatcher      *MockMatcher
	mockSuggester    *MockSuggester
	mockOutput       *MockOutput
	mockResponseType string
)

func setup() {
	mockRuleService = new(MockRuleService)
	mockScanner = new(MockScanner)
	mockMatcher = new(MockMatcher)
	mockSuggester = new(MockSuggester)
	mockOutput = new(MockOutput)
	mockResponseType = ""

}

// captureDebugLog captures the process's stderr, where the --debug logger
// writes.
func captureDebugLog(f func()) string {
	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	f()

	w.Close() //nolint:errcheck
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	os.Stderr = old
	return buf.String()
}

func TestHandler_Execute(t *testing.T) {
	setup()

	h := NewHandler(mockRuleService, mockScanner, mockSuggester, mockMatcher, mockOutput)

	config := config.Config{Dir: "test-dir", ExcludedCategories: []string{"dummy"}}
	artifacts := []*models.Artifact{{Name: "artifact1"}}
	findings := []*matcher.Finding{{Category: "Category1"}}
	suggestions := []*suggester.CategorySuggestion{{Name: "Suggestion1"}}

	mockScanner.On("Scan", config.Dir).Return(artifacts, nil)
	mockRuleService.On("GetCategoryRules", mock.Anything).Return([]*rules.Rule{}).Times(len(models.CategoryTitles))
	mockMatcher.On("Match", artifacts, []*rules.Rule{}).Return(findings).Times(len(models.CategoryTitles))
	mockSuggester.On("FindSuggestions", mock.Anything).Return(suggestions, nil)
	mockOutput.On("Response", mock.Anything).Return(nil)

	err := h.Execute(&config)

	assert.NoError(t, err)
	mockScanner.AssertExpectations(t)
	mockRuleService.AssertExpectations(t)
	mockMatcher.AssertExpectations(t)
	mockSuggester.AssertExpectations(t)
	mockOutput.AssertExpectations(t)
}

func TestHandler_Execute_ScanError(t *testing.T) {
	setup()

	h := NewHandler(mockRuleService, mockScanner, mockSuggester, mockMatcher, mockOutput)
	config := config.Config{Dir: "test-dir", ExcludedCategories: []string{"dummy"}}
	scanErr := errors.New("scan error")
	mockScanner.On("Scan", config.Dir).Return([]*models.Artifact{}, scanErr)

	err := h.Execute(&config)

	assert.Error(t, err)
	assert.Equal(t, scanErr, err)
	mockScanner.AssertExpectations(t)
}

func TestHandler_Execute_WithSuggestionsAndEnforce(t *testing.T) {
	setup()

	h := NewHandler(mockRuleService, mockScanner, mockSuggester, mockMatcher, mockOutput)

	config := config.Config{Dir: "test-dir", ExcludedCategories: []string{"dummy"}, Enforce: true}
	artifacts := []*models.Artifact{{Name: "artifact1"}}
	findings := []*matcher.Finding{{Category: "Category1"}}
	suggestions := []*suggester.CategorySuggestion{{Name: "Suggestion1"}}

	mockScanner.On("Scan", config.Dir).Return(artifacts, nil)
	mockRuleService.On("GetCategoryRules", mock.Anything).Return([]*rules.Rule{}).Times(len(models.CategoryTitles))
	mockMatcher.On("Match", artifacts, []*rules.Rule{}).Return(findings).Times(len(models.CategoryTitles))
	mockSuggester.On("FindSuggestions", mock.Anything, mock.Anything).Return(suggestions)
	mockOutput.On("Response", mock.Anything).Return(nil)

	err := h.Execute(&config)

	assert.Error(t, err)
	assert.IsType(t, &models.EnforceError{}, err)
	mockScanner.AssertExpectations(t)
	mockRuleService.AssertExpectations(t)
	mockMatcher.AssertExpectations(t)
	mockSuggester.AssertExpectations(t)
	mockOutput.AssertExpectations(t)
}

func TestHandler_Execute_DebugMode(t *testing.T) {
	setup()

	cfg := config.Config{
		Dir:                "test-dir",
		ExcludedCategories: []string{},
		Enforce:            false,
		Debug:              true,
	}
	artifacts := []*models.Artifact{{Name: "artifact1"}}
	findings := []*matcher.Finding{{Category: "Category1"}}
	suggestions := []*suggester.CategorySuggestion{{Name: "Suggestion1"}}

	mockScanner.On("Scan", cfg.Dir).Return(artifacts, nil)
	mockRuleService.On("GetCategoryRules", mock.Anything).Return([]*rules.Rule{}).Times(len(models.CategoryTitles))
	mockMatcher.On("Match", artifacts, []*rules.Rule{}).Return(findings).Times(len(models.CategoryTitles))
	mockSuggester.On("FindSuggestions", mock.Anything, mock.Anything).Return(suggestions)
	mockOutput.On("Response", mock.Anything).Return(nil)

	out := captureDebugLog(func() {
		err := NewHandler(mockRuleService, mockScanner, mockSuggester, mockMatcher, mockOutput).Execute(&cfg)
		assert.NoError(t, err)
	})

	// Debug output goes to stderr, so it cannot corrupt a report on stdout.
	assert.Contains(t, out, "Starting scan for directory:")
}

// newEnforcementHandler wires mocks reporting the given categories as uncovered.
func newEnforcementHandler(uncovered ...string) *Handler {
	suggestions := make([]*suggester.CategorySuggestion, 0, len(uncovered))
	for _, id := range uncovered {
		suggestions = append(suggestions, &suggester.CategorySuggestion{ID: id, Name: id})
	}

	mockRules := new(MockRuleService)
	mockRules.On("GetCategoryRules", mock.Anything).Return([]*rules.Rule{})

	mockScanner := new(MockScanner)
	mockScanner.On("Scan", mock.Anything).Return([]*models.Artifact{}, nil)

	mockMatcher := new(MockMatcher)
	mockMatcher.On("Match", mock.Anything, mock.Anything).Return([]*matcher.Finding{})

	mockSuggester := new(MockSuggester)
	mockSuggester.On("FindSuggestions", mock.Anything).Return(suggestions)

	mockOutput := new(MockOutput)
	mockOutput.On("Response", mock.Anything).Return(nil)

	return &Handler{
		RulesService:      mockRules,
		ScanService:       mockScanner,
		MatchService:      mockMatcher,
		SuggestionService: mockSuggester,
		OutputService:     mockOutput,
	}
}

func TestEnforcementDecision(t *testing.T) {
	tests := []struct {
		name      string
		uncovered []string
		cfg       config.Config
		wantFail  bool
		wantIn    string
	}{
		{
			name:      "no enforcement means gaps never fail",
			uncovered: []string{"SCA", "Coverage"},
			cfg:       config.Config{},
			wantFail:  false,
		},
		{
			name:      "enforce fails on any gap",
			uncovered: []string{"Coverage"},
			cfg:       config.Config{Enforce: true},
			wantFail:  true,
			wantIn:    "Coverage",
		},
		{
			name:      "enforce passes when nothing is uncovered",
			uncovered: nil,
			cfg:       config.Config{Enforce: true},
			wantFail:  false,
		},
		{
			// The point of --fail-on: block on security, not performance.
			name:      "fail-on limits enforcement to named categories",
			uncovered: []string{"SCA", "Performance Testing"},
			cfg:       config.Config{FailOn: []string{"SCA"}},
			wantFail:  true,
			wantIn:    "SCA",
		},
		{
			name:      "fail-on ignores categories it does not name",
			uncovered: []string{"Performance Testing"},
			cfg:       config.Config{FailOn: []string{"SCA"}},
			wantFail:  false,
		},
		{
			// config canonicalises FailOn, so the handler sees exact titles.
			name:      "fail-on matches the canonical title",
			uncovered: []string{"SCA"},
			cfg:       config.Config{FailOn: []string{"SCA"}},
			wantFail:  true,
		},
		{
			name:      "fail-on implies enforcement without --enforce",
			uncovered: []string{"SCA"},
			cfg:       config.Config{Enforce: false, FailOn: []string{"SCA"}},
			wantFail:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newEnforcementHandler(tt.uncovered...)
			cfg := tt.cfg
			cfg.Dir = t.TempDir()

			err := h.Execute(&cfg)
			if !tt.wantFail {
				assert.NoError(t, err)
				return
			}
			assert.Error(t, err)
			var enforceErr *models.EnforceError
			assert.True(t, errors.As(err, &enforceErr), "expected an EnforceError")
			if tt.wantIn != "" {
				assert.Contains(t, err.Error(), tt.wantIn)
			}
		})
	}
}

// A baseline lets enforcement be switched on before everything is fixed.
func TestBaselineSuppressesAcceptedGaps(t *testing.T) {
	path := filepath.Join(t.TempDir(), "baseline.yaml")
	assert.NoError(t, baseline.Write(path, []string{"SCA", "Coverage"}, nil))

	h := newEnforcementHandler("SCA", "Coverage")
	err := h.Execute(&config.Config{Dir: t.TempDir(), Enforce: true, Baseline: path})
	assert.NoError(t, err, "every gap is in the baseline, so the scan should pass")
}

func TestBaselineStillFailsOnNewGaps(t *testing.T) {
	path := filepath.Join(t.TempDir(), "baseline.yaml")
	assert.NoError(t, baseline.Write(path, []string{"SCA"}, nil))

	h := newEnforcementHandler("SCA", "Secrets Detection")
	err := h.Execute(&config.Config{Dir: t.TempDir(), Enforce: true, Baseline: path})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Secrets Detection")
	assert.NotContains(t, err.Error(), "SCA", "an accepted gap should not be reported as failing")
}

func TestWriteBaselineRecordsCurrentGapsAndPasses(t *testing.T) {
	path := filepath.Join(t.TempDir(), "baseline.yaml")

	h := newEnforcementHandler("SCA", "Coverage")
	err := h.Execute(&config.Config{Dir: t.TempDir(), Enforce: true, Baseline: path, WriteBaseline: true})
	assert.NoError(t, err, "writing a baseline should not fail the scan it is derived from")

	b, err := baseline.Load(path)
	assert.NoError(t, err)
	assert.Equal(t, []string{"Coverage", "SCA"}, b.Categories)
}

func TestBaselineLoadErrorIsSurfaced(t *testing.T) {
	path := filepath.Join(t.TempDir(), "baseline.yaml")
	assert.NoError(t, os.WriteFile(path, []byte("version: 99\n"), 0o600))

	h := newEnforcementHandler("SCA")
	err := h.Execute(&config.Config{Dir: t.TempDir(), Enforce: true, Baseline: path})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported version")
}

func TestWriteBaselineReportsWriteFailure(t *testing.T) {
	h := newEnforcementHandler("SCA")
	cfg := &config.Config{
		WriteBaseline: true,
		Baseline:      filepath.Join(t.TempDir(), "no-such-dir", baseline.DefaultPath),
	}

	err := h.Execute(cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to write baseline")
}

func TestSarifAnchor(t *testing.T) {
	tests := []struct {
		name      string
		dir       string
		artifacts []*models.Artifact
		want      string
	}{
		{
			name:      "repository-relative path",
			dir:       "/repo",
			artifacts: []*models.Artifact{{Location: "/repo/.github/workflows/ci.yml"}},
			want:      ".github/workflows/ci.yml",
		},
		{
			name:      "skips entries without a location",
			dir:       "/repo",
			artifacts: []*models.Artifact{{Location: ""}, {Location: "/repo/.gitlab-ci.yml"}},
			want:      ".gitlab-ci.yml",
		},
		{
			// A path outside the repository would not resolve for a consumer.
			name:      "skips paths outside the scanned directory",
			dir:       "/repo",
			artifacts: []*models.Artifact{{Location: "/elsewhere/ci.yml"}},
			want:      "",
		},
		{
			name:      "no artifacts",
			dir:       "/repo",
			artifacts: nil,
			want:      "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, sarifAnchor(tt.dir, tt.artifacts))
		})
	}
}

func TestHandler_Fix(t *testing.T) {
	h := newEnforcementHandler("Secrets Detection")
	h.SuggestionService = func() suggester.Suggester {
		m := new(MockSuggester)
		m.On("FindSuggestions", mock.Anything, mock.Anything, mock.Anything).Return(
			[]*suggester.CategorySuggestion{{
				ID: "Secrets Detection", Name: "Data Leakage & Secrets Detection",
				Suggestions: []*suggester.Suggestion{
					{Name: "Gitleaks", Repository: "https://github.com/gitleaks/gitleaks"},
				},
			}})
		return m
	}()

	dir := t.TempDir()
	assert.NoError(t, os.MkdirAll(filepath.Join(dir, ".github", "workflows"), 0o755))

	var buf bytes.Buffer
	assert.NoError(t, h.Fix(&config.Config{Dir: dir}, &buf))

	out := buf.String()
	assert.Contains(t, out, "is not covered")
	assert.Contains(t, out, ".github/workflows")
	assert.Contains(t, out, "gitleaks-action")
}

func TestHandler_FixWithNothingUncovered(t *testing.T) {
	h := newEnforcementHandler()

	var buf bytes.Buffer
	assert.NoError(t, h.Fix(&config.Config{Dir: t.TempDir()}, &buf))
	assert.Contains(t, buf.String(), "Nothing to fix")
}

func newFixHandler(categories ...string) *Handler {
	suggestions := make([]*suggester.CategorySuggestion, 0, len(categories))
	for _, id := range categories {
		suggestions = append(suggestions, &suggester.CategorySuggestion{
			ID: id, Name: id,
			Suggestions: []*suggester.Suggestion{
				{Name: "Gitleaks", Repository: "https://github.com/gitleaks/gitleaks"},
			},
		})
	}

	h := newEnforcementHandler()
	m := new(MockSuggester)
	m.On("FindSuggestions", mock.Anything, mock.Anything, mock.Anything).Return(suggestions)
	h.SuggestionService = m
	return h
}

func TestHandler_FixWrite(t *testing.T) {
	dir := t.TempDir()
	assert.NoError(t, os.MkdirAll(filepath.Join(dir, ".github", "workflows"), 0o755))

	var buf bytes.Buffer
	assert.NoError(t, newFixHandler("Secrets Detection").Fix(&config.Config{Dir: dir, Write: true}, &buf))

	assert.Contains(t, buf.String(), "Wrote .github/workflows/zanadir-suggested.yml")
	assert.Contains(t, buf.String(), "repeats checkout")

	written, err := os.ReadFile(filepath.Join(dir, ".github", "workflows", "zanadir-suggested.yml"))
	assert.NoError(t, err)
	assert.Contains(t, string(written), "gitleaks-action")
}

func TestHandler_FixWriteWithNothingToWrite(t *testing.T) {
	dir := t.TempDir()
	assert.NoError(t, os.MkdirAll(filepath.Join(dir, ".github", "workflows"), 0o755))

	var buf bytes.Buffer
	assert.NoError(t, newFixHandler().Fix(&config.Config{Dir: dir, Write: true}, &buf))

	assert.Contains(t, buf.String(), "Nothing to write")
	_, err := os.Stat(filepath.Join(dir, ".github", "workflows", "zanadir-suggested.yml"))
	assert.True(t, os.IsNotExist(err), "no file should be written when there is nothing to add")
}

func TestHandler_FixWriteRejectsNonGitHubPlatform(t *testing.T) {
	dir := t.TempDir()
	assert.NoError(t, os.WriteFile(filepath.Join(dir, ".gitlab-ci.yml"), []byte("x"), 0o600))

	var buf bytes.Buffer
	err := newFixHandler("Secrets Detection").Fix(&config.Config{Dir: dir, Write: true}, &buf)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "gitlab")
}

// The generated workflow names the tools it adds. Counting it as coverage made
// each regeneration delete the steps the previous one wrote.
func TestFixIgnoresItsOwnGeneratedWorkflow(t *testing.T) {
	own := &models.Artifact{Name: "zanadir-suggested", Location: "/repo/.github/workflows/zanadir-suggested.yml"}
	theirs := &models.Artifact{Name: "ci", Location: "/repo/.github/workflows/ci.yml"}

	mockRules := new(MockRuleService)
	mockRules.On("GetCategoryRules", mock.Anything).Return([]*rules.Rule{})

	mockScan := new(MockScanner)
	mockScan.On("Scan", mock.Anything).Return([]*models.Artifact{own, theirs}, nil)

	seen := make([][]*models.Artifact, 0)
	mockMatch := new(MockMatcher)
	mockMatch.On("Match", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		seen = append(seen, args.Get(0).([]*models.Artifact))
	}).Return([]*matcher.Finding{})

	mockSuggest := new(MockSuggester)
	mockSuggest.On("FindSuggestions", mock.Anything).Return([]*suggester.CategorySuggestion{})

	h := &Handler{
		RulesService: mockRules, ScanService: mockScan, MatchService: mockMatch,
		SuggestionService: mockSuggest, OutputService: new(MockOutput),
	}

	var buf bytes.Buffer
	assert.NoError(t, h.Fix(&config.Config{Dir: t.TempDir()}, &buf))

	assert.NotEmpty(t, seen)
	for _, batch := range seen {
		assert.Equal(t, []*models.Artifact{theirs}, batch,
			"the generated workflow must not be matched against")
	}
}

func TestExecuteMatchesEveryArtifact(t *testing.T) {
	own := &models.Artifact{Name: "zanadir-suggested", Location: "/repo/.github/workflows/zanadir-suggested.yml"}

	mockRules := new(MockRuleService)
	mockRules.On("GetCategoryRules", mock.Anything).Return([]*rules.Rule{})
	mockScan := new(MockScanner)
	mockScan.On("Scan", mock.Anything).Return([]*models.Artifact{own}, nil)

	var seen []*models.Artifact
	mockMatch := new(MockMatcher)
	mockMatch.On("Match", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		seen = args.Get(0).([]*models.Artifact)
	}).Return([]*matcher.Finding{})
	mockSuggest := new(MockSuggester)
	mockSuggest.On("FindSuggestions", mock.Anything).Return([]*suggester.CategorySuggestion{})
	mockOut := new(MockOutput)
	mockOut.On("Response", mock.Anything).Return(nil)

	h := &Handler{RulesService: mockRules, ScanService: mockScan, MatchService: mockMatch,
		SuggestionService: mockSuggest, OutputService: mockOut}

	assert.NoError(t, h.Execute(&config.Config{Dir: t.TempDir(), Output: "table"}))
	assert.Equal(t, []*models.Artifact{own}, seen,
		"a committed generated workflow is real coverage for scan")
}

func TestFixPropagatesScanError(t *testing.T) {
	boom := errors.New("not a git repository")
	mockScan := new(MockScanner)
	mockScan.On("Scan", mock.Anything).Return([]*models.Artifact{}, boom)

	h := &Handler{ScanService: mockScan, RulesService: new(MockRuleService)}

	var buf bytes.Buffer
	assert.ErrorIs(t, h.Fix(&config.Config{Dir: t.TempDir()}, &buf), boom)
}

func TestFixWriteSurfacesAWriteFailure(t *testing.T) {
	dir := t.TempDir()
	// .github/workflows occupied by a file, so the directory cannot be created.
	assert.NoError(t, os.MkdirAll(filepath.Join(dir, ".github"), 0o755))
	assert.NoError(t, os.WriteFile(filepath.Join(dir, ".github", "workflows"), []byte("x"), 0o600))

	var buf bytes.Buffer
	err := newFixHandler("Secrets Detection").Fix(&config.Config{Dir: dir, Write: true}, &buf)

	assert.Error(t, err)
}

func TestExecutePassesTheScoreToTheReportAndBadge(t *testing.T) {
	setup()

	h := NewHandler(mockRuleService, mockScanner, mockSuggester, mockMatcher, mockOutput)
	badgePath := filepath.Join(t.TempDir(), "badge.json")
	cfg := config.Config{Dir: "test-dir", Badge: badgePath}

	artifacts := []*models.Artifact{{Name: "artifact1"}}
	suggestions := []*suggester.CategorySuggestion{{ID: "SCA", Name: "SCA"}, {ID: "Linter", Name: "Linter"}}

	mockScanner.On("Scan", cfg.Dir).Return(artifacts, nil)
	mockRuleService.On("GetCategoryRules", mock.Anything).Return([]*rules.Rule{})
	mockMatcher.On("Match", artifacts, []*rules.Rule{}).Return([]*matcher.Finding{})
	mockSuggester.On("FindSuggestions", mock.Anything).Return(suggestions, nil)

	var reported output.Report
	mockOutput.On("Response", mock.Anything).Run(func(args mock.Arguments) {
		reported = args.Get(0).(output.Report)
	}).Return(nil)

	assert.NoError(t, h.Execute(&cfg))

	total := len(models.CategoryTitles)
	assert.Equal(t, score.Score{Covered: total - 2, Total: total}, reported.Score)

	data, err := os.ReadFile(badgePath)
	assert.NoError(t, err)
	assert.Contains(t, string(data), fmt.Sprintf(`"message": "%d/%d"`, total-2, total))
}

func TestExecuteSurfacesABadgeWriteFailure(t *testing.T) {
	setup()

	h := NewHandler(mockRuleService, mockScanner, mockSuggester, mockMatcher, mockOutput)
	cfg := config.Config{Dir: "test-dir", Badge: filepath.Join(t.TempDir(), "no-such-dir", "badge.json")}

	mockScanner.On("Scan", cfg.Dir).Return([]*models.Artifact{}, nil)
	mockRuleService.On("GetCategoryRules", mock.Anything).Return([]*rules.Rule{})
	mockMatcher.On("Match", mock.Anything, mock.Anything).Return([]*matcher.Finding{})
	mockSuggester.On("FindSuggestions", mock.Anything).Return([]*suggester.CategorySuggestion{}, nil)
	mockOutput.On("Response", mock.Anything).Return(nil)

	err := h.Execute(&cfg)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to write badge")
}

// A baseline records that a gap is accepted, not closed, so it must not move
// the score: a badge that turned green on a committed file would measure
// nothing.
func TestBaselineDoesNotChangeTheScore(t *testing.T) {
	setup()

	h := NewHandler(mockRuleService, mockScanner, mockSuggester, mockMatcher, mockOutput)
	path := filepath.Join(t.TempDir(), "baseline.yaml")
	assert.NoError(t, baseline.Write(path, []string{"SCA"}, nil))

	cfg := config.Config{Dir: "test-dir", Enforce: true, Baseline: path}
	suggestions := []*suggester.CategorySuggestion{{ID: "SCA", Name: "SCA"}}

	mockScanner.On("Scan", cfg.Dir).Return([]*models.Artifact{}, nil)
	mockRuleService.On("GetCategoryRules", mock.Anything).Return([]*rules.Rule{})
	mockMatcher.On("Match", mock.Anything, mock.Anything).Return([]*matcher.Finding{})
	mockSuggester.On("FindSuggestions", mock.Anything).Return(suggestions, nil)

	var reported output.Report
	mockOutput.On("Response", mock.Anything).Run(func(args mock.Arguments) {
		reported = args.Get(0).(output.Report)
	}).Return(nil)

	assert.NoError(t, h.Execute(&cfg))

	total := len(models.CategoryTitles)
	assert.Equal(t, score.Score{Covered: total - 1, Total: total}, reported.Score)
}

func weakFinding(category, location string, job *models.Job) *matcher.Finding {
	return &matcher.Finding{
		Category: category,
		Location: location,
		Artifact: &models.Artifact{
			Triggers: []models.Trigger{{Event: "pull_request"}},
			Location: location,
		},
		Job: job,
	}
}

func TestWeakenedReportsOnlyWhatItShould(t *testing.T) {
	coe := &models.Job{ContinueOnError: true}
	silent := func(string, ...interface{}) {}

	findings := []*matcher.Finding{
		weakFinding("Secrets Detection", "/repo/.github/workflows/ci.yml", coe),
		// Enforcing, so it must not be reported at all.
		weakFinding("SAST", "/repo/.github/workflows/sast.yml", &models.Job{}),
		// Unreadable, so the category stays quiet rather than being called weak.
		weakFinding("SCA", "/repo/.github/workflows/sca.yml", &models.Job{If: "always()"}),
	}

	weaknesses := weakened(nil, "/repo", findings, nil, silent)

	assert.Len(t, weaknesses, 1)
	assert.Equal(t, "Secrets Detection", weaknesses[0].Category)
	assert.Equal(t, "advisory", weaknesses[0].Verdict)
	// Reports point at repo-relative paths, not the scan directory's.
	assert.Equal(t, ".github/workflows/ci.yml", weaknesses[0].Location)
}

// Excluding a category says it does not apply; grading it anyway would argue
// with the operator.
func TestWeakenedSkipsExcludedCategories(t *testing.T) {
	coe := &models.Job{ContinueOnError: true}
	weaknesses := weakened([]string{"Secrets Detection"}, "/repo", []*matcher.Finding{
		weakFinding("Secrets Detection", "/repo/a.yml", coe),
		weakFinding("Linter", "/repo/b.yml", coe),
	}, nil, func(string, ...interface{}) {})

	assert.Len(t, weaknesses, 1)
	assert.Equal(t, "Linter", weaknesses[0].Category)
}

// Map iteration order is random, so without sorting the report would reshuffle
// between identical runs.
func TestWeakenedIsSorted(t *testing.T) {
	coe := &models.Job{ContinueOnError: true}
	weaknesses := weakened(nil, "/repo", []*matcher.Finding{
		weakFinding("Unit Tests", "/repo/c.yml", coe),
		weakFinding("Coverage", "/repo/a.yml", coe),
		weakFinding("Linter", "/repo/b.yml", coe),
	}, nil, func(string, ...interface{}) {})

	names := make([]string, 0, len(weaknesses))
	for _, weakness := range weaknesses {
		names = append(names, weakness.Category)
	}
	assert.Equal(t, []string{"Coverage", "Linter", "Unit Tests"}, names)
}

// A workflow outside the scan directory has no path a report can point at.
func TestWeakenedDropsLocationsOutsideTheScanDirectory(t *testing.T) {
	weaknesses := weakened(nil, "/repo", []*matcher.Finding{
		weakFinding("Linter", "/elsewhere/ci.yml", &models.Job{ContinueOnError: true}),
	}, nil, func(string, ...interface{}) {})

	assert.Len(t, weaknesses, 1)
	assert.Empty(t, weaknesses[0].Location)
}

// The scanner can hand back a nil artifact, and the anchor has to step over it
// rather than dereference it.
func TestSarifAnchorSkipsNilArtifacts(t *testing.T) {
	anchor := sarifAnchor("/repo", []*models.Artifact{
		nil,
		{Location: "/repo/.github/workflows/ci.yml"},
	})
	assert.Equal(t, ".github/workflows/ci.yml", anchor)
}

func TestSarifAnchorWithNothingUsable(t *testing.T) {
	assert.Empty(t, sarifAnchor("/repo", []*models.Artifact{nil, {Location: "/elsewhere/ci.yml"}, {}}))
}

// Code scanning rejects a result without a location, so a repository with no
// CI file zanadir could read still gets one: the README, else the first file.
func TestSarifAnchorFallsBackToAFileThatExists(t *testing.T) {
	dir := testrepo.Write(t, map[string]string{"go.mod": "module x\n", "README.md": "# x\n", ".github/workflows/bad.yml": "on: ["})
	skipped := &models.Artifact{Location: filepath.Join(dir, ".github", "workflows", "bad.yml"), Skipped: "not valid YAML"}
	assert.Equal(t, "README.md", sarifAnchor(dir, []*models.Artifact{skipped}), "a file that was skipped is not an anchor")

	dir = testrepo.Write(t, map[string]string{"go.mod": "module x\n", "main.go": "package main\n"})
	assert.Equal(t, "go.mod", sarifAnchor(dir, nil))
}

// Setup builds the handler the CLI actually runs with, from the embedded rules
// and suggestions. Nothing exercised it, so a bad embed would have surfaced
// only at runtime.
func TestSetup(t *testing.T) {
	h, err := Setup()
	assert.NoError(t, err)
	assert.NotNil(t, h.RulesService)
	assert.NotNil(t, h.ScanService)
	assert.NotNil(t, h.MatchService)
	assert.NotNil(t, h.SuggestionService)
	assert.NotNil(t, h.OutputService)
}

// The workflow fix --write generates is its own output, not the repository's,
// so the second run, which finds it, must write the same file as the first.
func TestFixWriteIsIdempotent(t *testing.T) {
	h, err := Setup()
	require.NoError(t, err)
	dir := testrepo.Write(t, map[string]string{"go.mod": "module x\n", ".git/HEAD": "ref: refs/heads/main\n"})
	workflow := filepath.Join(dir, ".github", "workflows", "zanadir-suggested.yml")

	var written []string
	for range 2 {
		require.NoError(t, h.Fix(&config.Config{Dir: dir, Write: true}, io.Discard))
		data, err := os.ReadFile(workflow)
		require.NoError(t, err)
		written = append(written, string(data))
	}
	assert.Equal(t, written[0], written[1])
	assert.NotContains(t, written[0], "zizmor", "Workflow Security applies once the repository has workflows of its own")
}

// A category both in --fail-on and excluded can never fail, which the
// operator is told; an excluded category is not also called not applicable.
func TestExecuteWarnsOfFailOnExcludedAndLeavesExclusionsOutOfNotApplicable(t *testing.T) {
	warnings := captureStderr(t)
	h, err := Setup()
	require.NoError(t, err)
	dir := testrepo.Write(t, map[string]string{"go.mod": "module x\n", ".git/HEAD": "ref: refs/heads/main\n"})
	report := filepath.Join(t.TempDir(), "report.json")

	err = h.Execute(&config.Config{Dir: dir, Output: config.OutputJSON, OutputFile: report,
		ExcludedCategories: []string{"SAST", "IaC Security"}, FailOn: []string{"SAST"}})
	require.NoError(t, err)

	assert.Contains(t, warnings.String(), "Warning: SAST is both in --fail-on and excluded, so it can never fail the scan")
	data, err := os.ReadFile(report)
	require.NoError(t, err)
	assert.NotContains(t, string(data), "IaC Security")
	assert.Contains(t, string(data), "Workflow Security", "a category that is not applicable, and not excluded, is still named")
}

func TestSetupRejectsABadCustomRule(t *testing.T) {
	_, err := Setup(rules.FileRule{ID: "acme"})
	assert.ErrorContains(t, err, "acme")
}

func TestFixInDebugMode(t *testing.T) {
	setup()
	h := NewHandler(mockRuleService, mockScanner, mockSuggester, mockMatcher, mockOutput)
	cfg := config.Config{Dir: t.TempDir(), Debug: true}

	mockScanner.On("Scan", cfg.Dir).Return([]*models.Artifact{}, nil)
	mockRuleService.On("GetCategoryRules", mock.Anything).Return([]*rules.Rule{})
	mockMatcher.On("Match", mock.Anything, mock.Anything).Return([]*matcher.Finding{})
	mockSuggester.On("FindSuggestions", mock.Anything).Return([]*suggester.CategorySuggestion{}, nil)

	var buf bytes.Buffer
	assert.NoError(t, h.Fix(&cfg, &buf))
}

func TestExecuteSurfacesAnOutputFailureInDebugMode(t *testing.T) {
	setup()
	h := NewHandler(mockRuleService, mockScanner, mockSuggester, mockMatcher, mockOutput)
	cfg := config.Config{Dir: "test-dir", Debug: true}
	boom := errors.New("disk full")

	mockScanner.On("Scan", cfg.Dir).Return([]*models.Artifact{}, nil)
	mockRuleService.On("GetCategoryRules", mock.Anything).Return([]*rules.Rule{})
	mockMatcher.On("Match", mock.Anything, mock.Anything).Return([]*matcher.Finding{})
	mockSuggester.On("FindSuggestions", mock.Anything).Return([]*suggester.CategorySuggestion{}, nil)
	mockOutput.On("Response", mock.Anything).Return(boom)

	assert.ErrorIs(t, h.Execute(&cfg), boom)
}

func weakenedCfg(flags func(*config.Config)) *config.Config {
	cfg := &config.Config{Dir: "/repo"}
	flags(cfg)
	return cfg
}

// Turning weak controls into failures overnight would break people, so it
// takes its own flag and --enforce alone does not do it.
func TestEnforcementSeparatesUncoveredFromWeakened(t *testing.T) {
	h := &Handler{}
	silent := func(string, ...interface{}) {}
	uncovered := []string{"SCA"}
	weaknesses := []output.Weakness{{Category: "Secrets Detection", Verdict: "advisory"}}

	tests := []struct {
		name              string
		cfg               *config.Config
		expectedUncovered []string
		expectedWeakened  []string
	}{
		{
			name: "nothing asked for",
			cfg:  weakenedCfg(func(c *config.Config) {}),
		},
		{
			name:              "enforce alone leaves weakened controls alone",
			cfg:               weakenedCfg(func(c *config.Config) { c.Enforce = true }),
			expectedUncovered: []string{"SCA"},
		},
		{
			name:             "fail-on-weakened alone leaves uncovered alone",
			cfg:              weakenedCfg(func(c *config.Config) { c.FailOnWeakened = true }),
			expectedWeakened: []string{"Secrets Detection (advisory)"},
		},
		{
			name:              "both",
			cfg:               weakenedCfg(func(c *config.Config) { c.Enforce = true; c.FailOnWeakened = true }),
			expectedUncovered: []string{"SCA"},
			expectedWeakened:  []string{"Secrets Detection (advisory)"},
		},
		{
			name: "fail-on narrows weakened too",
			cfg: weakenedCfg(func(c *config.Config) {
				c.FailOnWeakened = true
				c.FailOn = []string{"SAST"}
			}),
		},
		{
			name: "fail-on naming the weakened category",
			cfg: weakenedCfg(func(c *config.Config) {
				c.FailOnWeakened = true
				c.FailOn = []string{"Secrets Detection"}
			}),
			expectedWeakened: []string{"Secrets Detection (advisory)"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := h.enforceableCategories(tt.cfg, uncovered, weaknesses, silent)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedUncovered, result.uncovered)
			assert.Equal(t, tt.expectedWeakened, result.weakened)
		})
	}
}

// Accepting a weakness must not also accept it getting worse later, or the
// baseline would quietly absorb a degradation of the thing it accepted.
func TestBaselineAcceptsAWeaknessButNotItsDegradation(t *testing.T) {
	h := &Handler{}
	silent := func(string, ...interface{}) {}
	path := filepath.Join(t.TempDir(), "base.yaml")
	assert.NoError(t, baseline.Write(path, nil, map[string]string{"Secrets Detection": "partial"}))

	cfg := &config.Config{Dir: "/repo", FailOnWeakened: true, Baseline: path}

	atPartial, err := h.enforceableCategories(cfg, nil,
		[]output.Weakness{{Category: "Secrets Detection", Verdict: "partial"}}, silent)
	assert.NoError(t, err)
	assert.True(t, atPartial.empty(), "the verdict it was accepted at must still pass")

	degraded, err := h.enforceableCategories(cfg, nil,
		[]output.Weakness{{Category: "Secrets Detection", Verdict: "advisory"}}, silent)
	assert.NoError(t, err)
	assert.Equal(t, []string{"Secrets Detection (advisory)"}, degraded.weakened)
}

func TestEnforcementErrorNamesBothKinds(t *testing.T) {
	both := enforcement{uncovered: []string{"SCA"}, weakened: []string{"Linter (advisory)"}}
	assert.Equal(t,
		"uncovered categories: SCA; controls that cannot fail a build: Linter (advisory)",
		both.Error())

	assert.Equal(t, "uncovered categories: SCA", enforcement{uncovered: []string{"SCA"}}.Error())
	assert.Equal(t, "controls that cannot fail a build: Linter (advisory)",
		enforcement{weakened: []string{"Linter (advisory)"}}.Error())
	assert.Empty(t, enforcement{}.Error())
}

// --write-baseline has to record the verdicts, or the baseline it writes could
// not tell a degradation from the state it accepted.
func TestWriteBaselineRecordsWeakenedVerdicts(t *testing.T) {
	setup()
	h := NewHandler(mockRuleService, mockScanner, mockSuggester, mockMatcher, mockOutput)
	path := filepath.Join(t.TempDir(), "base.yaml")
	cfg := config.Config{Dir: "test-dir", Baseline: path, WriteBaseline: true}

	artifact := &models.Artifact{
		Triggers: []models.Trigger{{Event: "pull_request"}},
		Location: "test-dir/ci.yml",
		Jobs:     []*models.Job{{ContinueOnError: true}},
	}
	finding := &matcher.Finding{Category: "Secrets Detection", Location: "test-dir/ci.yml", Artifact: artifact, Job: artifact.Jobs[0]}

	mockScanner.On("Scan", cfg.Dir).Return([]*models.Artifact{artifact}, nil)
	mockRuleService.On("GetCategoryRules", mock.Anything).Return([]*rules.Rule{})
	mockMatcher.On("Match", mock.Anything, mock.Anything).Return([]*matcher.Finding{finding})
	mockSuggester.On("FindSuggestions", mock.Anything).Return([]*suggester.CategorySuggestion{}, nil)
	mockOutput.On("Response", mock.Anything).Return(nil)

	assert.NoError(t, h.Execute(&cfg))

	loaded, err := baseline.Load(path)
	assert.NoError(t, err)
	verdict, ok := loaded.AcceptedVerdict("Secrets Detection")
	assert.True(t, ok)
	assert.Equal(t, "advisory", verdict)
}

// The report says which CI systems were read, so a repository whose CI was
// found but matched no tool is told apart from one with no CI at all.
func TestExecuteReportsThePlatformsFound(t *testing.T) {
	mockRules := new(MockRuleService)
	mockRules.On("GetCategoryRules", mock.Anything).Return([]*rules.Rule{})
	mockScan := new(MockScanner)
	mockScan.On("Scan", mock.Anything).Return([]*models.Artifact{}, nil)
	mockMatch := new(MockMatcher)
	mockMatch.On("Match", mock.Anything, mock.Anything).Return([]*matcher.Finding{})
	mockSuggest := new(MockSuggester)
	mockSuggest.On("FindSuggestions", mock.Anything).Return([]*suggester.CategorySuggestion{})
	var reported output.Report
	mockOut := new(MockOutput)
	mockOut.On("Response", mock.Anything).Run(func(args mock.Arguments) {
		reported = args.Get(0).(output.Report)
	}).Return(nil)

	dir := testrepo.Write(t, map[string]string{".gitlab-ci.yml": "test:\n  script: [make]\n", "Jenkinsfile": "sh 'make'\n"})
	h := &Handler{RulesService: mockRules, ScanService: mockScan, MatchService: mockMatch,
		SuggestionService: mockSuggest, OutputService: mockOut}

	assert.NoError(t, h.Execute(&config.Config{Dir: dir, Output: "json"}))
	assert.Equal(t, []string{"gitlab", "jenkins"}, reported.Platforms)
}

func TestExecuteWarnsAboutASkippedFile(t *testing.T) {
	var warnings bytes.Buffer
	stderr = &warnings
	t.Cleanup(func() { stderr = os.Stderr })

	dir := t.TempDir()
	broken := &models.Artifact{Location: filepath.Join(dir, ".github", "workflows", "broken.yml"),
		Skipped: "it is not valid YAML (yaml: line 2: did not find expected key), so the CI platform cannot run it either"}
	mockRules := new(MockRuleService)
	mockRules.On("GetCategoryRules", mock.Anything).Return([]*rules.Rule{})
	mockScan := new(MockScanner)
	mockScan.On("Scan", mock.Anything).Return([]*models.Artifact{broken, {Location: filepath.Join(dir, "ok.yml")}}, nil)
	mockMatch := new(MockMatcher)
	mockMatch.On("Match", mock.Anything, mock.Anything).Return([]*matcher.Finding{})
	mockSuggest := new(MockSuggester)
	mockSuggest.On("FindSuggestions", mock.Anything).Return([]*suggester.CategorySuggestion{})
	mockOut := new(MockOutput)
	mockOut.On("Response", mock.Anything).Return(nil)

	h := &Handler{RulesService: mockRules, ScanService: mockScan, MatchService: mockMatch,
		SuggestionService: mockSuggest, OutputService: mockOut}
	assert.NoError(t, h.Execute(&config.Config{Dir: dir, Output: "json"}))
	assert.Equal(t, "Warning: skipped .github/workflows/broken.yml: it is not valid YAML "+
		"(yaml: line 2: did not find expected key), so the CI platform cannot run it either\n", warnings.String())
}

func fakeGate(t *testing.T, merge *gate.Gate, err error) *[]remote.Repository {
	t.Helper()
	var asked []remote.Repository
	old := readGate
	readGate = func(repo remote.Repository, _ string) (*gate.Gate, error) {
		asked = append(asked, repo)
		return merge, err
	}
	t.Cleanup(func() { readGate = old })
	return &asked
}

func captureStderr(t *testing.T) *bytes.Buffer {
	t.Helper()
	var warnings bytes.Buffer
	stderr = &warnings
	t.Cleanup(func() { stderr = os.Stderr })
	return &warnings
}

func TestMergeGate(t *testing.T) {
	artifacts := []*models.Artifact{{Jobs: []*models.Job{{Check: "Unit tests"}, {}}}}
	repo := &remote.Repository{Owner: "o", Name: "r"}

	asked := fakeGate(t, nil, nil)
	assert.Nil(t, mergeGate(&config.Config{Repo: repo}, artifacts), "off unless asked for")
	assert.Empty(t, *asked)

	merge := &gate.Gate{Branch: "main", Checks: []gate.Check{{Context: "Unit tests", AppID: 15368}, {Context: "e2e", AppID: 15368}}}
	asked = fakeGate(t, merge, nil)
	assert.Same(t, merge, mergeGate(&config.Config{Repo: repo, RequiredChecks: true}, artifacts))
	assert.Equal(t, []remote.Repository{*repo}, *asked)
	assert.Equal(t, []string{`required check "e2e" is reported by no workflow job, so pull requests wait for it`}, merge.Problems)

	checkout := testrepo.Write(t, map[string]string{".git/config": "[remote \"origin\"]\n\turl = git@github.com:acme/app.git\n"})
	asked = fakeGate(t, &gate.Gate{}, nil)
	assert.NotNil(t, mergeGate(&config.Config{Dir: checkout, RequiredChecks: true}, nil))
	assert.Equal(t, "github.com/acme/app", (*asked)[0].String(), "a checkout is read through its origin")

	warnings := captureStderr(t)
	fakeGate(t, nil, errors.New("rate limited"))
	assert.Nil(t, mergeGate(&config.Config{Repo: repo, RequiredChecks: true}, nil))
	assert.Nil(t, mergeGate(&config.Config{Dir: t.TempDir(), RequiredChecks: true}, nil))
	assert.Contains(t, warnings.String(), "Warning: --required-checks could not read the merge settings (rate limited); grading without them\n")
	assert.Contains(t, warnings.String(), ".git/config")
}

func TestReportGate(t *testing.T) {
	assert.Nil(t, reportGate(nil))
	assert.Equal(t, &output.Gate{Branch: "main", Required: []string{"DCO", "lint"}, Scanning: []string{"zizmor"}, Problems: []string{"p"}},
		reportGate(&gate.Gate{Branch: "main", Checks: []gate.Check{{Context: "DCO"}, {Context: "lint"}, {Context: "DCO", AppID: 1}}, Scanning: []string{"zizmor"}, Problems: []string{"p"}}))
}

func TestExecuteGradesAgainstTheMergeSettings(t *testing.T) {
	fakeGate(t, &gate.Gate{Branch: "main"}, nil)
	dir := t.TempDir()
	lint := &models.Artifact{Location: filepath.Join(dir, "ci.yml"), Triggers: []models.Trigger{{Event: "pull_request"}},
		Jobs: []*models.Job{{Check: "lint", Run: "golangci-lint run"}}}

	mockRules := new(MockRuleService)
	mockRules.On("GetCategoryRules", mock.Anything).Return([]*rules.Rule{})
	mockScan := new(MockScanner)
	mockScan.On("Scan", mock.Anything).Return([]*models.Artifact{lint}, nil)
	mockMatch := new(MockMatcher)
	mockMatch.On("Match", mock.Anything, mock.Anything).Return([]*matcher.Finding{
		{Category: "Linter", Location: lint.Location, Artifact: lint, Job: lint.Jobs[0]}})
	mockSuggest := new(MockSuggester)
	mockSuggest.On("FindSuggestions", mock.Anything).Return([]*suggester.CategorySuggestion{})
	var got output.Report
	mockOut := new(MockOutput)
	mockOut.On("Response", mock.Anything).Run(func(args mock.Arguments) { got = args.Get(0).(output.Report) }).Return(nil)

	h := &Handler{RulesService: mockRules, ScanService: mockScan, MatchService: mockMatch,
		SuggestionService: mockSuggest, OutputService: mockOut}
	assert.NoError(t, h.Execute(&config.Config{Dir: dir, Output: "json", RequiredChecks: true, Repo: &remote.Repository{Owner: "o", Name: "r"}}))
	assert.Equal(t, &output.Gate{Branch: "main", Required: []string{}}, got.Gate)
	assert.Contains(t, got.Weakened, output.Weakness{Category: "Linter", Verdict: "partial", Location: "ci.yml", Reason: grading.NotRequired})
}
