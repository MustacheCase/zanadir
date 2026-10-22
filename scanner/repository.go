package scanner

import (
	"github.com/MustacheCase/zanadir/models"
	"github.com/MustacheCase/zanadir/parser"
	"github.com/MustacheCase/zanadir/wrapper"
)

// RepositoryScanner parses a repository's CI configuration and follows the build tools its jobs call.
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

	// A step that runs "make lint" runs whatever the lint target does.
	wrappers := wrapper.New(root)
	for _, a := range artifacts {
		for _, job := range a.Jobs {
			if wrapped := wrappers.Expand(job.Run); wrapped != "" {
				job.Run += "\n" + wrapped
			}
		}
	}
	return artifacts, nil
}

// NewRepositoryScanner returns a Scanner that reads every supported platform and the dependency bots.
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
