package scanner

import (
	"path/filepath"

	"github.com/MustacheCase/zanadir/models"
	"github.com/MustacheCase/zanadir/parser"
)

type ciParser struct {
	Path   string
	Parser parser.Parser
}

type RepositoryScanner struct {
	ciParsers []ciParser
}

// Scan collects artifacts from every parser that finds something, not just the
// first. Returning early meant a repository using two platforms, or one whose
// dependency updates are declared outside its pipeline, had the rest of its
// configuration go unread.
func (r *RepositoryScanner) Scan(repositoryDir string) ([]*models.Artifact, error) {
	var artifacts []*models.Artifact

	for _, cp := range r.ciParsers {
		path := filepath.Join(repositoryDir, cp.Path)
		if !cp.Parser.Exists(path) {
			continue
		}
		parsed, err := cp.Parser.Parse(path)
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, parsed...)
	}

	return artifacts, nil
}

func NewRepositoryScanner() Scanner {
	githubParser := parser.NewGithubParser()
	circleCIParser := parser.NewCircleCIParser()
	gitlabParser := parser.NewGitlabParser()
	botParser := parser.NewDependencyBotParser()

	return &RepositoryScanner{
		ciParsers: []ciParser{
			{Path: "/.github/workflows/", Parser: githubParser},
			{Path: "/.circleci/", Parser: circleCIParser},
			{Path: "/.gitlab-ci.yml", Parser: gitlabParser},
			// Rooted at the repository: these configs have several possible
			// names and the parser knows them.
			{Path: "/", Parser: botParser},
		},
	}
}
