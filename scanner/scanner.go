package scanner

import "github.com/MustacheCase/zanadir/models"

// Scanner interface
type Scanner interface {
	Scan(dir string) ([]*models.Artifact, error)
}
