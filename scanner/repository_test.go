package scanner

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/MustacheCase/zanadir/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type MockParser struct {
	mock.Mock
}

func (m *MockParser) Exists(path string) bool {
	args := m.Called(path)
	return args.Bool(0)
}

func (m *MockParser) Parse(path string) ([]*models.Artifact, error) {
	args := m.Called(path)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.Artifact), args.Error(1)
}

var (
	mockParser  *MockParser
	mockScanner = new(MockScanner)
)

func setup() {
	mockParser = new(MockParser)
	mockScanner = new(MockScanner)
}

func TestRepositoryScanner_Scan(t *testing.T) {
	setup()

	repoDir := "/test/repo"
	ciPath := filepath.Join(repoDir, "/.github/workflows/")

	mockParser.On("Exists", ciPath).Return(true)
	mockParser.On("Parse", ciPath).Return([]*models.Artifact{{Name: "artifact1"}}, nil)

	s := NewRepositoryScanner(mockParser)
	artifacts, err := s.Scan(repoDir)

	assert.NoError(t, err)
	assert.NotNil(t, artifacts)
	assert.Len(t, artifacts, 1)
	assert.Equal(t, "artifact1", artifacts[0].Name)

	mockParser.AssertExpectations(t)
}

func TestRepositoryScanner_Scan_NoCI(t *testing.T) {
	setup()

	repoDir := "/test/repo"
	ciPath := filepath.Join(repoDir, "/.github/workflows/")

	mockParser.On("Exists", ciPath).Return(false)

	s := NewRepositoryScanner(mockParser)
	artifacts, err := s.Scan(repoDir)

	assert.NoError(t, err)
	assert.Nil(t, artifacts)

	mockParser.AssertExpectations(t)
}

func TestRepositoryScanner_Scan_ParseError(t *testing.T) {
	setup()

	repoDir := "/test/repo"
	ciPath := filepath.Join(repoDir, "/.github/workflows/")

	mockParser.On("Exists", ciPath).Return(true)
	mockParser.On("Parse", ciPath).Return(nil, errors.New("parse error"))

	s := NewRepositoryScanner(mockParser)
	artifacts, err := s.Scan(repoDir)

	assert.Error(t, err)
	assert.Nil(t, artifacts)

	mockParser.AssertExpectations(t)
}
