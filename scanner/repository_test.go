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
	mockParser *MockParser
)

func setup() {
	mockParser = new(MockParser)
}

func TestRepositoryScanner_Scan(t *testing.T) {
	setup()

	repoDir := "/test/repo"
	githubCIPath := filepath.Join(repoDir, "/.github/workflows/")
	circleCIPath := filepath.Join(repoDir, "/.circleci/")

	mockParser.On("Exists", githubCIPath).Return(false) // Github doesn't exist
	mockParser.On("Exists", circleCIPath).Return(true)  // CircleCI exists
	mockParser.On("Parse", circleCIPath).Return([]*models.Artifact{{Name: "artifact1"}}, nil)

	s := &RepositoryScanner{
		ciParsers: map[int]ciParser{
			githubCI: {Path: "/.github/workflows/", Parser: mockParser},
			circleCI: {Path: "/.circleci/", Parser: mockParser},
		},
	}
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
	githubCIPath := filepath.Join(repoDir, "/.github/workflows/")
	circleCIPath := filepath.Join(repoDir, "/.circleci/")

	mockParser.On("Exists", githubCIPath).Return(false)
	mockParser.On("Exists", circleCIPath).Return(false)

	s := &RepositoryScanner{
		ciParsers: map[int]ciParser{
			githubCI: {Path: "/.github/workflows/", Parser: mockParser},
			circleCI: {Path: "/.circleci/", Parser: mockParser},
		},
	}
	artifacts, err := s.Scan(repoDir)

	assert.NoError(t, err)
	assert.Nil(t, artifacts)

	mockParser.AssertExpectations(t)
}

func TestRepositoryScanner_Scan_ParseError(t *testing.T) {
	setup()

	repoDir := "/test/repo"
	githubCIPath := filepath.Join(repoDir, "/.github/workflows/")
	circleCIPath := filepath.Join(repoDir, "/.circleci/")

	mockParser.On("Exists", githubCIPath).Return(false)
	mockParser.On("Exists", circleCIPath).Return(true)
	mockParser.On("Parse", circleCIPath).Return(nil, errors.New("parse error"))

	s := &RepositoryScanner{
		ciParsers: map[int]ciParser{
			githubCI: {Path: "/.github/workflows/", Parser: mockParser},
			circleCI: {Path: "/.circleci/", Parser: mockParser},
		},
	}
	artifacts, err := s.Scan(repoDir)

	assert.Error(t, err)
	assert.Nil(t, artifacts)

	mockParser.AssertExpectations(t)
}
