package suggester_test

import (
	"strings"
	"testing"

	"github.com/MustacheCase/zanadir/matcher"
	"github.com/MustacheCase/zanadir/models"
	"github.com/MustacheCase/zanadir/suggester"
	"github.com/stretchr/testify/assert"
)

func TestFindSuggestions(t *testing.T) {
	s, err := suggester.NewSuggestionService()
	assert.NoError(t, err)

	// Define a dummy finding that does not cover "SCA" and "Secrets".
	findings := []*matcher.Finding{
		{Category: "OtherCategory"},
	}
	// Expect that the embedded suggestions (e.g. "SCA", "Secrets") are suggested.
	result := s.FindSuggestions(findings, []string{}, nil)
	assert.NotEmpty(t, result, "expected non-empty suggestions")
	// Optionally, verify that known IDs exist based on your embedded suggestions content.
}

func TestFindSuggestionsWithExclusion(t *testing.T) {
	s, err := suggester.NewSuggestionService()
	assert.NoError(t, err)

	// Define findings covering one category only.
	findings := []*matcher.Finding{
		{Category: "SCA"},
	}
	// Exclude "Secrets" (or any other known category as defined in your embedded suggestions).
	result := s.FindSuggestions(findings, []string{"Secrets"}, nil)
	// Verify that no suggested category has an ID equal to "Secrets".
	for _, cat := range result {
		assert.NotEqual(t, "Secrets", cat.ID)
	}
}

// Regression test: a suggestions.yaml id that does not match its CategoryTitle
// silently drops the category from every scan.
func TestFindSuggestionsCoversEveryCategory(t *testing.T) {
	s, err := suggester.NewSuggestionService()
	assert.NoError(t, err)

	result := s.FindSuggestions(nil, nil, nil)

	got := make(map[string]bool, len(result))
	for _, cat := range result {
		got[cat.ID] = true
	}

	assert.Len(t, result, len(models.CategoryTitles))
	for _, title := range models.CategoryTitles {
		assert.True(t, got[string(title)], "category %q was never suggested", title)
	}
}

func TestFindSuggestionsExcludesByCanonicalTitle(t *testing.T) {
	s, err := suggester.NewSuggestionService()
	assert.NoError(t, err)

	result := s.FindSuggestions(nil, []string{string(models.Secrets)}, nil)

	for _, cat := range result {
		assert.NotEqual(t, string(models.Secrets), cat.ID)
	}
	assert.Len(t, result, len(models.CategoryTitles)-1)
}

func TestSuggestionServiceValidatesCategories(t *testing.T) {
	_, err := suggester.NewSuggestionService()
	assert.NoError(t, err, "every models.CategoryTitle must exist in suggestions.yaml")
}

// findCategory returns the suggested category with the given id, if present.
func findCategory(cats []*suggester.CategorySuggestion, id string) *suggester.CategorySuggestion {
	for _, c := range cats {
		if c.ID == id {
			return c
		}
	}
	return nil
}

func toolNames(cat *suggester.CategorySuggestion) []string {
	names := make([]string, 0, len(cat.Suggestions))
	for _, s := range cat.Suggestions {
		names = append(names, s.Name)
	}
	return names
}

// A Go repository has no use for ESLint, Pylint or RuboCop.
func TestFindSuggestionsFiltersByLanguage(t *testing.T) {
	s, err := suggester.NewSuggestionService()
	assert.NoError(t, err)

	result := s.FindSuggestions(nil, nil, []string{"Go"})

	linter := findCategory(result, "Linter")
	assert.NotNil(t, linter)
	names := toolNames(linter)
	assert.Contains(t, names, "GolangCI-Lint")
	assert.Contains(t, names, "OxSecurity MegaLinter", "language-agnostic tools are always kept")
	assert.NotContains(t, names, "ESLint")
	assert.NotContains(t, names, "Pylint")
	assert.NotContains(t, names, "RuboCop")
}

func TestFindSuggestionsKeepsAllToolsWhenNoLanguageDetected(t *testing.T) {
	s, err := suggester.NewSuggestionService()
	assert.NoError(t, err)

	result := s.FindSuggestions(nil, nil, nil)

	linter := findCategory(result, "Linter")
	assert.NotNil(t, linter)
	names := toolNames(linter)
	assert.Contains(t, names, "ESLint")
	assert.Contains(t, names, "GolangCI-Lint")
	assert.Contains(t, names, "Pylint")
}

func TestFindSuggestionsPolyglotKeepsEachLanguage(t *testing.T) {
	s, err := suggester.NewSuggestionService()
	assert.NoError(t, err)

	result := s.FindSuggestions(nil, nil, []string{"Go", "Python"})

	names := toolNames(findCategory(result, "Linter"))
	assert.Contains(t, names, "GolangCI-Lint")
	assert.Contains(t, names, "Pylint")
	assert.Contains(t, names, "Flake8")
	assert.NotContains(t, names, "ESLint")
}

// A category whose tools all target other languages must still name something.
func TestFindSuggestionsFallsBackWhenFilterEmptiesCategory(t *testing.T) {
	s, err := suggester.NewSuggestionService()
	assert.NoError(t, err)

	// Rust matches none of the Unit Tests tools (JUnit, pytest, Mocha).
	result := s.FindSuggestions(nil, nil, []string{"Rust"})

	unitTests := findCategory(result, "Unit Tests")
	assert.NotNil(t, unitTests)
	assert.NotEmpty(t, unitTests.Suggestions, "category should not be reported with an empty tool list")
}

// FindSuggestions must not narrow the shared catalogue it reads from.
func TestFindSuggestionsDoesNotMutateSharedCatalogue(t *testing.T) {
	s, err := suggester.NewSuggestionService()
	assert.NoError(t, err)

	goOnly := s.FindSuggestions(nil, nil, []string{"Go"})
	assert.NotContains(t, toolNames(findCategory(goOnly, "Linter")), "ESLint")

	// A second call with no language must still see the full catalogue.
	unfiltered := s.FindSuggestions(nil, nil, nil)
	assert.Contains(t, toolNames(findCategory(unfiltered, "Linter")), "ESLint")
}

// The catalogue is the entire user-facing text of a scan, and a copy-pasted
// description silently misdescribes a category on every run — Performance
// Testing shipped with Code Coverage's wording. Assert the text is distinct
// and complete so the next paste is caught here rather than by a user.
func TestCatalogueDescriptionsAreDistinctAndComplete(t *testing.T) {
	s, err := suggester.NewSuggestionService()
	assert.NoError(t, err)

	all := s.FindSuggestions(nil, []string{}, nil)
	assert.Len(t, all, len(models.CategoryTitles), "every category should be suggested when nothing is covered")

	seen := make(map[string]string, len(all))
	for _, c := range all {
		assert.NotEmpty(t, c.Description, "category %q has no description", c.ID)
		assert.True(t, strings.HasSuffix(c.Description, "."),
			"category %q description should end in a period: %q", c.ID, c.Description)

		if other, dup := seen[c.Description]; dup {
			t.Errorf("categories %q and %q share a description: %q", other, c.ID, c.Description)
		}
		seen[c.Description] = c.ID

		for _, tool := range c.Suggestions {
			assert.NotEmpty(t, tool.Name, "category %q has a tool with no name", c.ID)
			assert.NotEmpty(t, tool.Description, "tool %q in %q has no description", tool.Name, c.ID)
			assert.NotEmpty(t, tool.Repository, "tool %q in %q has no repository", tool.Name, c.ID)
		}
	}
}
