package handler

import (
	"errors"
	"testing"

	"github.com/MustacheCase/zanadir/matcher"
	"github.com/MustacheCase/zanadir/models"
	"github.com/MustacheCase/zanadir/rules"
	"github.com/MustacheCase/zanadir/storage"
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

func (m *MockSuggester) FindSuggestions(findings []*matcher.Finding) []*storage.CategorySuggestion {
	args := m.Called(findings)
	return args.Get(0).([]*storage.CategorySuggestion)
}

func (m *MockOutput) Response(suggestions []*storage.CategorySuggestion) error {
	args := m.Called(suggestions)
	return args.Error(0)
}

var (
	mockRuleService *MockRuleService
	mockScanner     *MockScanner
	mockMatcher     *MockMatcher
	mockSuggester   *MockSuggester
	mockOutput      *MockOutput
)

func setup() {
	mockRuleService = new(MockRuleService)
	mockScanner = new(MockScanner)
	mockMatcher = new(MockMatcher)
	mockSuggester = new(MockSuggester)
	mockOutput = new(MockOutput)

}

func TestHandler_Execute(t *testing.T) {
	setup()

	h := NewHandler(mockRuleService, mockScanner, mockSuggester, mockMatcher, mockOutput)

	dir := "test-dir"
	artifacts := []*models.Artifact{{Name: "artifact1"}}
	findings := []*matcher.Finding{{Category: "Category1"}}
	suggestions := []*storage.CategorySuggestion{{Name: "Suggestion1"}}

	mockScanner.On("Scan", dir).Return(artifacts, nil)
	mockRuleService.On("GetCategoryRules", mock.Anything).Return([]*rules.Rule{}).Twice()
	mockMatcher.On("Match", artifacts, []*rules.Rule{}).Return(findings).Twice()
	mockSuggester.On("FindSuggestions", mock.Anything).Return(suggestions, nil)
	mockOutput.On("Response", suggestions).Return(nil)

	err := h.Execute(dir)

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
	dir := "test-dir"
	scanErr := errors.New("scan error")
	mockScanner.On("Scan", dir).Return([]*models.Artifact{}, scanErr)

	err := h.Execute(dir)

	assert.Error(t, err)
	assert.Equal(t, scanErr, err)
	mockScanner.AssertExpectations(t)
}
