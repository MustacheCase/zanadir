package scanner

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/MustacheCase/zanadir/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type MockScanner struct {
	mock.Mock
}

var (
	mockScanner = new(MockScanner)
)

func (m *MockScanner) Scan(dir string) ([]*models.Artifact, error) {
	args := m.Called(dir)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.Artifact), args.Error(1)
}

func TestScan_ValidRepository(t *testing.T) {
	setup()

	dir := t.TempDir()
	_ = os.Mkdir(filepath.Join(dir, ".git"), 0755)

	mockScanner = new(MockScanner)
	mockScanner.On("Scan", dir).Return([]*models.Artifact{}, nil)

	svc := NewScanService(mockScanner)
	artifacts, err := svc.Scan(dir)

	assert.NoError(t, err)
	assert.NotNil(t, artifacts)
	mockScanner.AssertExpectations(t)
}

func TestScan_NotARepository(t *testing.T) {
	setup()

	dir := t.TempDir()

	mockScanner = new(MockScanner)
	svc := NewScanService(mockScanner)
	artifacts, err := svc.Scan(dir)

	assert.Error(t, err)
	assert.Nil(t, artifacts)
	assert.Equal(t, "not a git repository", err.Error())
}

func TestScan_ScannerError(t *testing.T) {
	setup()

	dir := t.TempDir()
	_ = os.Mkdir(filepath.Join(dir, ".git"), 0755)

	mockScanner = new(MockScanner)
	mockScanner.On("Scan", dir).Return(nil, errors.New("scanner error"))

	svc := NewScanService(mockScanner)
	artifacts, err := svc.Scan(dir)

	assert.Error(t, err)
	assert.Nil(t, artifacts)
	assert.Equal(t, "scanner error", err.Error())
	mockScanner.AssertExpectations(t)
}
