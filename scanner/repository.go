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

// Scan collects artifacts from every parser that finds something. Stopping at
// the first left a repository using two platforms half read.
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
	return &RepositoryScanner{
		ciParsers: []ciParser{
			{Path: "/.github/workflows/", Parser: parser.NewGithubParser()},
			{Path: "/.circleci/", Parser: parser.NewCircleCIParser()},
			// The parsers take the directory to look in, not the file.
			{Path: "/", Parser: parser.NewGitlabParser()},
			{Path: "/", Parser: parser.NewBitbucketParser()},
			// Rooted at the repository: these configs sit outside any CI
			// directory and have several possible names.
			{Path: "/", Parser: parser.NewDependencyBotParser()},
		},
	}
}
