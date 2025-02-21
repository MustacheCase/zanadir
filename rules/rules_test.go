package rules

import (
	"errors"
	"testing"

	"github.com/MustacheCase/zanadir/models"
	"github.com/MustacheCase/zanadir/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockStorage mocks the storage interface
type MockStorage struct {
	mock.Mock
}

func (m *MockStorage) ReadCategoriesSuggestions() ([]storage.CategorySuggestion, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]storage.CategorySuggestion), args.Error(1)
}

func (m *MockStorage) ReadRules() ([]storage.FileRule, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]storage.FileRule), args.Error(1)
}

var (
	mockStorage *MockStorage
)

func setup() {
	mockStorage = new(MockStorage)
}

func TestGetCategoryRules(t *testing.T) {
	setup()

	mockStorage.On("ReadRules").Return([]storage.FileRule{
		{
			ID:         "rule-1",
			ApplyOn:    []string{"file1", "file2"},
			Categories: []string{"category-a"},
			Regex:      ".*\\.txt",
		},
		{
			ID:         "rule-2",
			ApplyOn:    []string{"file3"},
			Categories: []string{"category-b"},
			Regex:      ".*\\.go",
		},
	}, nil)

	ruleService, err := NewRulesService(mockStorage)
	assert.NoError(t, err)

	categoryARules := ruleService.GetCategoryRules(models.CategoryTitle("category-a"))
	assert.Len(t, categoryARules, 1)
	assert.Equal(t, "rule-1", categoryARules[0].ID)

	categoryBRules := ruleService.GetCategoryRules(models.CategoryTitle("category-b"))
	assert.Len(t, categoryBRules, 1)
	assert.Equal(t, "rule-2", categoryBRules[0].ID)

	categoryCRules := ruleService.GetCategoryRules(models.CategoryTitle("category-c"))
	assert.Empty(t, categoryCRules)
}

func TestConvertRules(t *testing.T) {
	setup()

	service := &service{storageService: mockStorage}

	fileRules := []storage.FileRule{
		{
			ID:         "rule-1",
			ApplyOn:    []string{"file1"},
			Categories: []string{"cat-1"},
			Regex:      ".*\\.txt",
		},
	}

	convertedRules := service.convertRules(fileRules)
	assert.Len(t, convertedRules, 1)
	assert.Equal(t, "rule-1", convertedRules[0].ID)
	assert.Equal(t, []string{"file1"}, convertedRules[0].ApplyOn)
	assert.Equal(t, []string{"cat-1"}, convertedRules[0].Categories)
	assert.True(t, convertedRules[0].Regex.MatchString("test.txt"))
}

func TestCreateRulesCollection(t *testing.T) {
	setup()

	mockStorage.On("ReadRules").Return([]storage.FileRule{
		{
			ID:         "rule-1",
			ApplyOn:    []string{"file1"},
			Categories: []string{"cat-1"},
			Regex:      ".*\\.txt",
		},
		{
			ID:         "rule-2",
			ApplyOn:    []string{"file2"},
			Categories: []string{"cat-2"},
			Regex:      ".*\\.go",
		},
	}, nil)

	service := &service{storageService: mockStorage}
	collection, err := service.createRulesCollection()
	assert.NoError(t, err)
	assert.NotNil(t, collection)

	assert.Len(t, collection.ByCategory["cat-1"], 1)
	assert.Equal(t, "rule-1", collection.ByCategory["cat-1"][0].ID)

	assert.Len(t, collection.ByCategory["cat-2"], 1)
	assert.Equal(t, "rule-2", collection.ByCategory["cat-2"][0].ID)

	assert.Equal(t, "rule-1", collection.ByID["rule-1"].ID)
	assert.Equal(t, "rule-2", collection.ByID["rule-2"].ID)
}

func TestCreateRulesCollectionError(t *testing.T) {
	setup()

	mockStorage.On("ReadRules").Return(nil, errors.New("storage error"))

	service := &service{storageService: mockStorage}
	collection, err := service.createRulesCollection()

	assert.Error(t, err)
	assert.Nil(t, collection)
	assert.Equal(t, "storage error", err.Error())
}
