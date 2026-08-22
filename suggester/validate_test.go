package suggester

import (
	"testing"

	"github.com/MustacheCase/zanadir/models"
	"github.com/stretchr/testify/assert"
)

// fullCatalogue returns one entry per known category, which is what a valid
// suggestions.yaml provides.
func fullCatalogue() []CategorySuggestion {
	cats := make([]CategorySuggestion, 0, len(models.CategoryTitles))
	for _, title := range models.CategoryTitles {
		cats = append(cats, CategorySuggestion{ID: string(title), Name: string(title)})
	}
	return cats
}

func TestValidateCategoriesMapAcceptsFullCatalogue(t *testing.T) {
	assert.NoError(t, validateCategoriesMap(buildCategoriesMap(fullCatalogue())))
}

// The embedded catalogue is always valid, so the failure path is only
// reachable by constructing a catalogue that is missing an entry. This is the
// exact shape of the bug the validation exists to catch: an id that does not
// match its CategoryTitle.
func TestValidateCategoriesMapReportsMissingCategory(t *testing.T) {
	cats := fullCatalogue()
	// Rename one entry to the stale id that shipped in suggestions.yaml.
	for i := range cats {
		if cats[i].ID == string(models.Secrets) {
			cats[i].ID = "Secrets"
		}
	}

	err := validateCategoriesMap(buildCategoriesMap(cats))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), string(models.Secrets))
}

func TestValidateCategoriesMapReportsEveryMissingCategory(t *testing.T) {
	err := validateCategoriesMap(buildCategoriesMap(nil))
	assert.Error(t, err)
	for _, title := range models.CategoryTitles {
		assert.Contains(t, err.Error(), string(title))
	}
}

func TestNewServiceRejectsIncompleteCatalogue(t *testing.T) {
	s, err := newService(fullCatalogue()[1:])
	assert.Error(t, err)
	assert.Nil(t, s)
}

func TestNewServiceAcceptsFullCatalogue(t *testing.T) {
	s, err := newService(fullCatalogue())
	assert.NoError(t, err)
	assert.NotNil(t, s)
	assert.Len(t, s.CategoriesMap, len(models.CategoryTitles))
}
