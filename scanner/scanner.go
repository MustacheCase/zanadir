package scanner

import (
	"errors"
	"path/filepath"

	"github.com/MustacheCase/zanadir/models"
	"github.com/MustacheCase/zanadir/utils"
)

// Scanner finds the CI configuration in a directory.
type Scanner interface {
	Scan(dir string) ([]*models.Artifact, error)
}

// service refuses anything that is not a git repository before handing it to
// the repository scanner.
type service struct {
	repository Scanner
}

func (s *service) Scan(dir string) ([]*models.Artifact, error) {
	// A worktree or a submodule has a .git file pointing at its git directory.
	if git := filepath.Join(dir, ".git"); !utils.IsDir(git) && !utils.IsFile(git) {
		return nil, errors.New("not a git repository")
	}
	return s.repository.Scan(dir)
}

func NewScanService(repository Scanner) Scanner {
	return &service{repository: repository}
}
