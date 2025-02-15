package scanner

import (
	"path/filepath"

	"github.com/MustacheCase/zanadir/models"
	"github.com/MustacheCase/zanadir/parser"
)

const (
	githubCI = iota
)

type ciParser struct {
	Path   string
	Parser parser.Parser
}

var ciParsers = map[int]ciParser{
	githubCI: {Path: "/.github/workflows/", Parser: parser.NewGithubParser()},
}

type RepositoryScanner struct{}

func (r *RepositoryScanner) Scan(repositoryDir string) ([]*models.Artifact, error) {
	for _, cp := range ciParsers {
		path := filepath.Join(repositoryDir, cp.Path)
		if !cp.Parser.Exists(path) {
			// print log and continue
			continue
		}
		artifacts, err := cp.Parser.Parse(path)
		if err != nil {
			return nil, err
		}
		return artifacts, nil
	}
	// print log which we didn't find any ci actions
	return nil, nil
}
