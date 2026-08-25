package handler

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/MustacheCase/zanadir/baseline"
	"github.com/MustacheCase/zanadir/config"
	"github.com/MustacheCase/zanadir/matcher"
	"github.com/MustacheCase/zanadir/models"
	"github.com/MustacheCase/zanadir/output"
	"github.com/MustacheCase/zanadir/rules"
	"github.com/MustacheCase/zanadir/suggester"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
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

func (m *MockSuggester) FindSuggestions(findings []*matcher.Finding, excludedCategories []string, languages []string) []*suggester.CategorySuggestion {
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

func captureOutput(f func()) string {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	f()

	w.Close() //nolint:errcheck
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	os.Stdout = old
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

	out := captureOutput(func() {
		err := NewHandler(mockRuleService, mockScanner, mockSuggester, mockMatcher, mockOutput).Execute(&cfg)
		assert.NoError(t, err)
	})

	// Check that the debug output contains our expected log message.
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
	assert.NoError(t, baseline.Write(path, []string{"SCA", "Coverage"}))

	h := newEnforcementHandler("SCA", "Coverage")
	err := h.Execute(&config.Config{Dir: t.TempDir(), Enforce: true, Baseline: path})
	assert.NoError(t, err, "every gap is in the baseline, so the scan should pass")
}

func TestBaselineStillFailsOnNewGaps(t *testing.T) {
	path := filepath.Join(t.TempDir(), "baseline.yaml")
	assert.NoError(t, baseline.Write(path, []string{"SCA"}))

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
