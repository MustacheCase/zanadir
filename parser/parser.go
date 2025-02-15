package parser

import "github.com/MustacheCase/zanadir/models"

// Parser interface
type Parser interface {
	Parse(location string) ([]*models.Artifact, error)
	Exists(location string) bool
}
