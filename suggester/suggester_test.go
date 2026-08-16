package suggester_test

import (
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
	result := s.FindSuggestions(findings, []string{})
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
	result := s.FindSuggestions(findings, []string{"Secrets"})
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

	result := s.FindSuggestions(nil, nil)

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

	result := s.FindSuggestions(nil, []string{string(models.Secrets)})

	for _, cat := range result {
		assert.NotEqual(t, string(models.Secrets), cat.ID)
	}
	assert.Len(t, result, len(models.CategoryTitles)-1)
}

func TestSuggestionServiceValidatesCategories(t *testing.T) {
	_, err := suggester.NewSuggestionService()
	assert.NoError(t, err, "every models.CategoryTitle must exist in suggestions.yaml")
}
