package suggester_test

import (
	"errors"
	"testing"

	"github.com/MustacheCase/zanadir/matcher"
	"github.com/MustacheCase/zanadir/storage"
	"github.com/MustacheCase/zanadir/suggester"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

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
	return args.Get(0).([]storage.FileRule), args.Error(1)
}

var (
	mockStorage *MockStorage
)

func setup() {
	mockStorage = new(MockStorage)
}

func TestFindSuggestions(t *testing.T) {
	setup()

	mockStorage.On("ReadCategoriesSuggestions").Return([]storage.CategorySuggestion{
		{ID: "SCA", Name: "Category 1"},
		{ID: "Secrets", Name: "Category 2"},
	}, nil)

	suggesterService, err := suggester.NewSuggestionService(mockStorage)
	assert.NoError(t, err)

	tests := []struct {
		name           string
		findings       []*matcher.Finding
		expectedResult []*storage.CategorySuggestion
	}{
		{
			name:     "No findings - all categories suggested",
			findings: []*matcher.Finding{},
			expectedResult: []*storage.CategorySuggestion{
				{ID: "SCA", Name: "Category 1"},
				{ID: "Secrets", Name: "Category 2"},
			},
		},
		{
			name: "One category covered - suggest missing category",
			findings: []*matcher.Finding{
				{Category: "SCA"},
			},
			expectedResult: []*storage.CategorySuggestion{
				{ID: "Secrets", Name: "Category 2"},
			},
		},
		{
			name: "All categories covered - no suggestions",
			findings: []*matcher.Finding{
				{Category: "SCA"},
				{Category: "Secrets"},
			},
			expectedResult: []*storage.CategorySuggestion{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := suggesterService.FindSuggestions(tt.findings, []string{})
			assert.ElementsMatch(t, tt.expectedResult, result)
		})
	}
}

func TestFindSuggestionsWithExclusion(t *testing.T) {
	setup()

	mockStorage.On("ReadCategoriesSuggestions").Return([]storage.CategorySuggestion{
		{ID: "SCA", Name: "Category 1"},
		{ID: "Secrets", Name: "Category 2"},
	}, nil)

	suggesterService, err := suggester.NewSuggestionService(mockStorage)
	assert.NoError(t, err)

	tests := []struct {
		name           string
		findings       []*matcher.Finding
		expectedResult []*storage.CategorySuggestion
	}{
		{
			name:     "No findings - all categories suggested",
			findings: []*matcher.Finding{},
			expectedResult: []*storage.CategorySuggestion{
				{ID: "SCA", Name: "Category 1"},
			},
		},
		{
			name: "One category covered - suggest missing category",
			findings: []*matcher.Finding{
				{Category: "SCA"},
			},
			expectedResult: []*storage.CategorySuggestion{},
		},
		{
			name: "All categories covered - no suggestions",
			findings: []*matcher.Finding{
				{Category: "SCA"},
			},
			expectedResult: []*storage.CategorySuggestion{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := suggesterService.FindSuggestions(tt.findings, []string{"Secrets"})
			assert.ElementsMatch(t, tt.expectedResult, result)
		})
	}
}

func TestNewSuggestionService_Error(t *testing.T) {
	setup()

	mockStorage.On("ReadCategoriesSuggestions").Return(nil, errors.New("storage error"))

	_, err := suggester.NewSuggestionService(mockStorage)
	assert.Error(t, err)
}
