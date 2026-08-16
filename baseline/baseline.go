package baseline

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// DefaultPath is used when --baseline is given without a path.
const DefaultPath = ".zanadir-baseline.yaml"

// Version is the baseline schema version.
const Version = 1

// Baseline records categories a repository knowingly does not cover. They are
// still reported, but do not fail a scan; only gaps appearing later do.
type Baseline struct {
	Version    int      `yaml:"version"`
	Categories []string `yaml:"categories"`
}

// Load reads a baseline. A missing file means nothing has been accepted yet.
func Load(path string) (*Baseline, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path is operator-supplied
	if os.IsNotExist(err) {
		return &Baseline{Version: Version}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read baseline %s: %w", path, err)
	}

	var b Baseline
	if err := yaml.Unmarshal(data, &b); err != nil {
		return nil, fmt.Errorf("failed to parse baseline %s: %w", path, err)
	}
	if b.Version != Version {
		return nil, fmt.Errorf("baseline %s has unsupported version %d, expected %d", path, b.Version, Version)
	}
	return &b, nil
}

// Write saves the given categories as the accepted baseline.
func Write(path string, categories []string) error {
	sorted := append([]string(nil), categories...)
	sort.Strings(sorted)

	b := Baseline{Version: Version, Categories: sorted}
	data, err := yaml.Marshal(b)
	if err != nil {
		return fmt.Errorf("failed to marshal baseline: %w", err)
	}

	header := "# zanadir baseline: categories that are uncovered but accepted.\n" +
		"# These are still reported; they just do not fail a scan.\n" +
		"# Regenerate with: zanadir scan --dir . --write-baseline\n"

	if err := os.WriteFile(path, []byte(header+string(data)), 0o600); err != nil {
		return fmt.Errorf("failed to write baseline %s: %w", path, err)
	}
	return nil
}

// Contains reports whether a category has been accepted.
func (b *Baseline) Contains(category string) bool {
	if b == nil {
		return false
	}
	for _, c := range b.Categories {
		if strings.EqualFold(strings.TrimSpace(c), category) {
			return true
		}
	}
	return false
}
