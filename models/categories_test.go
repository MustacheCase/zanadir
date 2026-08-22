package models_test

import (
	"testing"

	"github.com/MustacheCase/zanadir/models"
	"github.com/stretchr/testify/assert"
)

func TestResolveCategory(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected models.CategoryTitle
		ok       bool
	}{
		{name: "canonical", input: "Secrets Detection", expected: models.Secrets, ok: true},
		{name: "lowercase", input: "secrets detection", expected: models.Secrets, ok: true},
		{name: "whitespace", input: "  SCA  ", expected: models.SCA, ok: true},
		{name: "stale id", input: "Secrets", ok: false},
		{name: "typo", input: "Lintr", ok: false},
		{name: "empty", input: "", ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := models.ResolveCategory(tt.input)
			assert.Equal(t, tt.ok, ok)
			if tt.ok {
				assert.Equal(t, tt.expected, got)
			}
		})
	}
}

func TestResolveCategoryAcceptsEveryTitle(t *testing.T) {
	for _, title := range models.CategoryTitles {
		got, ok := models.ResolveCategory(string(title))
		assert.True(t, ok, "%q should resolve", title)
		assert.Equal(t, title, got)
	}
}

func TestCategoryNamesMatchesTitles(t *testing.T) {
	assert.Len(t, models.CategoryNames(), len(models.CategoryTitles))
	for i, title := range models.CategoryTitles {
		assert.Equal(t, string(title), models.CategoryNames()[i])
	}
}
