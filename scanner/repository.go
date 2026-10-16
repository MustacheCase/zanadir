package scanner

import (
	"github.com/MustacheCase/zanadir/models"
	"github.com/MustacheCase/zanadir/parser"
)

type RepositoryScanner struct {
	parsers []parser.Parser
}

// Scan collects artifacts from every parser that finds something. Stopping at
// the first left a repository using two platforms half read.
func (r *RepositoryScanner) Scan(root string) ([]*models.Artifact, error) {
	var artifacts []*models.Artifact
	for _, p := range r.parsers {
		if !p.Exists(root) {
			continue
		}
		parsed, err := p.Parse(root)
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, parsed...)
	}
	return artifacts, nil
}

func NewRepositoryScanner() Scanner {
	parsers := make([]parser.Parser, 0, len(parser.Platforms)+1)
	for _, platform := range parser.Platforms {
		parsers = append(parsers, platform.Parser)
	}
	// The dependency bots declare themselves in a configuration file rather
	// than a pipeline, so they are read alongside whichever platform is used.
	parsers = append(parsers, parser.NewDependencyBotParser())
	return &RepositoryScanner{parsers: parsers}
}
