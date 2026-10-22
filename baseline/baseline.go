// Package baseline reads and writes the file of uncovered categories a repository has accepted.
package baseline

import (
	"fmt"
	"maps"
	"os"
	"slices"
	"sort"
	"strings"

	"github.com/MustacheCase/zanadir/grading"
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
	// Weakened records the verdict each accepted weakness had when it was
	// accepted, so a control getting worse later is not accepted with it.
	Weakened map[string]string `yaml:"weakened,omitempty"`
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
	// A verdict nothing ranks below would accept any regression, so one
	// that is not a verdict is refused rather than compared.
	for _, category := range slices.Sorted(maps.Keys(b.Weakened)) {
		verdict := grading.Verdict(strings.ToLower(strings.TrimSpace(b.Weakened[category])))
		if !slices.Contains(verdicts, verdict) {
			return nil, fmt.Errorf("baseline %s accepts %s at %q, which is not a verdict (%s)",
				path, category, b.Weakened[category], strings.Join(names(verdicts), ", "))
		}
		b.Weakened[category] = string(verdict)
	}
	return &b, nil
}

var verdicts = []grading.Verdict{grading.Enforcing, grading.Partial, grading.Advisory, grading.Unknown}

func names(vs []grading.Verdict) []string {
	out := make([]string, len(vs))
	for i, v := range vs {
		out[i] = string(v)
	}
	return out
}

// Write saves the given categories, and the verdicts of any accepted
// weaknesses, as the baseline.
func Write(path string, categories []string, weakened map[string]string) error {
	sorted := append([]string(nil), categories...)
	sort.Strings(sorted)

	b := Baseline{Version: Version, Categories: sorted, Weakened: weakened}
	data, err := yaml.Marshal(b)
	if err != nil {
		return fmt.Errorf("failed to marshal baseline: %w", err)
	}

	header := "# zanadir baseline: categories that are uncovered but accepted,\n" +
		"# and weakened controls accepted at the verdict recorded here.\n" +
		"# These are still reported; they just do not fail a scan. A weakened\n" +
		"# control that gets worse than its recorded verdict fails again.\n" +
		"# Regenerate with: zanadir scan --dir . --write-baseline\n"

	if err := os.WriteFile(path, []byte(header+string(data)), 0o600); err != nil {
		return fmt.Errorf("failed to write baseline %s: %w", path, err)
	}
	return nil
}

// AcceptedVerdict returns the verdict a weakened category was accepted at, and
// whether it was accepted at all.
func (b *Baseline) AcceptedVerdict(category string) (string, bool) {
	if b == nil {
		return "", false
	}
	for recorded, verdict := range b.Weakened {
		if strings.EqualFold(strings.TrimSpace(recorded), category) {
			return verdict, true
		}
	}
	return "", false
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
