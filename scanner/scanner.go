package scanner

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/MustacheCase/zanadir/models"
)

const (
	repositoryScanner = iota
)

type ciScanner struct {
	Type    int
	Scanner Scanner
}

var ciScanners = map[int]ciScanner{
	repositoryScanner: {Type: repositoryScanner, Scanner: NewRepositoryScanner()},
}

// Scanner interface
type Scanner interface {
	Scan(dir string) ([]*models.Artifact, error)
}

type service struct{}

func (s *service) Scan(dir string) ([]*models.Artifact, error) {
	if s.isRepository(dir) {
		return ciScanners[repositoryScanner].Scanner.Scan(dir)
	}
	return nil, errors.New("not a git repository")
}

func (s *service) isRepository(dir string) bool {
	gitPath := filepath.Join(dir, ".git")
	info, err := os.Stat(gitPath)
	if err != nil {
		return false
	}
	return info.IsDir()
}

func NewScanService() Scanner {
	return &service{}
}
