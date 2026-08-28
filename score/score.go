// Package score expresses a scan result as category coverage: how many of the
// categories that apply to a repository have tooling behind them.
package score

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/MustacheCase/zanadir/models"
)

// Score is a repository's category coverage.
type Score struct {
	Covered int
	Total   int
}

// Of returns the coverage for a scan. Excluded categories leave the
// denominator, because a repository should not be marked down for a category
// it has declared irrelevant. Baseline-accepted gaps do not leave it: a
// baseline records that a gap is tolerated, not that it was closed, and a
// score that rose when someone committed a file would measure nothing.
func Of(excluded []string, uncovered int) Score {
	skip := make(map[string]bool, len(excluded))
	for _, c := range excluded {
		if title, ok := models.ResolveCategory(c); ok {
			skip[string(title)] = true
		}
	}

	total := 0
	for _, title := range models.CategoryTitles {
		if !skip[string(title)] {
			total++
		}
	}

	covered := total - uncovered
	if covered < 0 {
		covered = 0
	}
	return Score{Covered: covered, Total: total}
}

// String renders the score the way it reads on a badge.
func (s Score) String() string {
	if s.Total == 0 {
		return "n/a"
	}
	return fmt.Sprintf("%d/%d", s.Covered, s.Total)
}

// Percent is the covered share, rounded down.
func (s Score) Percent() int {
	if s.Total == 0 {
		return 0
	}
	return s.Covered * 100 / s.Total
}

func (s Score) colour() string {
	switch {
	case s.Total == 0:
		return "lightgrey"
	case s.Covered == s.Total:
		return "brightgreen"
	case s.Percent() >= 80:
		return "green"
	case s.Percent() >= 60:
		return "yellow"
	case s.Percent() >= 40:
		return "orange"
	default:
		return "red"
	}
}

// endpoint is the shields.io endpoint badge schema.
type endpoint struct {
	SchemaVersion int    `json:"schemaVersion"`
	Label         string `json:"label"`
	Message       string `json:"message"`
	Color         string `json:"color"`
}

// Write saves the score as a shields.io endpoint badge file, for a README to
// point at once CI publishes it.
func Write(path string, s Score) error {
	data, err := json.MarshalIndent(endpoint{
		SchemaVersion: 1,
		Label:         "zanadir",
		Message:       s.String(),
		Color:         s.colour(),
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal badge: %w", err)
	}

	// 0644: the badge is served publicly, and the step that publishes it may
	// run as a different user than the scan.
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil { //nolint:gosec // operator-supplied path; a badge is world-readable by design
		return fmt.Errorf("failed to write badge %s: %w", path, err)
	}
	return nil
}
