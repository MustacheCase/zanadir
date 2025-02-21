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

// Scanner interface
type Scanner interface {
	Scan(dir string) ([]*models.Artifact, error)
}

type service struct {
	CIScanner map[int]ciScanner
}

func (s *service) Scan(dir string) ([]*models.Artifact, error) {
	if s.isRepository(dir) {
		return s.CIScanner[repositoryScanner].Scanner.Scan(dir)
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

func NewScanService(repoScanner Scanner) Scanner {
	return &service{
		CIScanner: map[int]ciScanner{
			repositoryScanner: {Type: repositoryScanner, Scanner: repoScanner},
		},
	}
}
