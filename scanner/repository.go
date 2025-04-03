package scanner

import (
	"path/filepath"

	"github.com/MustacheCase/zanadir/models"
	"github.com/MustacheCase/zanadir/parser"
)

const (
	githubCI = iota
	circleCI
	gitlabCI
)

type ciParser struct {
	Path   string
	Parser parser.Parser
}

type RepositoryScanner struct {
	ciParsers map[int]ciParser
}

func (r *RepositoryScanner) Scan(repositoryDir string) ([]*models.Artifact, error) {
	for _, cp := range r.ciParsers {
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

func NewRepositoryScanner() Scanner {
	githubParser := parser.NewGithubParser()
	circleCIParser := parser.NewCircleCIParser()

	return &RepositoryScanner{
		ciParsers: map[int]ciParser{
			githubCI: {Path: "/.github/workflows/", Parser: githubParser},
			circleCI: {Path: "/.circleci/", Parser: circleCIParser},
			gitlabCI: {Path: "/.gitlab-ci.yml", Parser: parser.NewGitlabParser()},
		},
	}
}
