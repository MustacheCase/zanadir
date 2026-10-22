// Package parser reads the CI configuration of each supported platform into Artifacts.
package parser

import "github.com/MustacheCase/zanadir/models"

// Parser reads one kind of CI configuration. Both methods take the
// repository root; where the configuration lives inside it is the parser's
// business.
type Parser interface {
	Parse(root string) ([]*models.Artifact, error)
	Exists(root string) bool
}
