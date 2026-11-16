package parser

import (
	"path/filepath"
	"strings"

	"github.com/MustacheCase/zanadir/internal/repofs"
	"github.com/MustacheCase/zanadir/models"
)

// CodeScanningParser reads code scanning's default setup, which runs CodeQL
// in a workflow GitHub generates. Only a fetch sees it, through the API, and
// records it in models.CodeScanningFile.
type CodeScanningParser struct{}

func (CodeScanningParser) read(root string) ([]byte, bool) {
	data, err := repofs.ReadFile(root, filepath.Join(root, filepath.FromSlash(models.CodeScanningFile)))
	return data, err == nil
}

// Exists reports whether a fetch found default setup on.
func (p CodeScanningParser) Exists(root string) bool {
	_, ok := p.read(root)
	return ok
}

// Parse reports default setup as the CodeQL step it runs on pushes and pull
// requests to the default branch and weekly. It has no file to point to.
func (p CodeScanningParser) Parse(root string) ([]*models.Artifact, error) {
	data, ok := p.read(root)
	if !ok {
		return nil, nil
	}
	return []*models.Artifact{{
		Name:     "Code scanning default setup",
		Triggers: []models.Trigger{{Event: "push"}, {Event: "pull_request"}, {Event: "schedule"}},
		Jobs:     []*models.Job{{Name: "Analyze", Run: codeqlCommand(strings.Join(strings.Fields(string(data)), ","))}},
	}}, nil
}

// NewCodeScanningParser returns a Parser for code scanning's default setup.
func NewCodeScanningParser() Parser {
	return CodeScanningParser{}
}
