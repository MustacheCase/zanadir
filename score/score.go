// Package score expresses a scan result as category coverage.
package score

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/MustacheCase/zanadir/models"
)

// Score is how many of the applicable categories are covered.
type Score struct {
	Covered int
	Total   int
}

// Of returns the coverage for a scan. Excluded categories leave the
// denominator; baseline-accepted gaps do not.
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

func (s Score) String() string {
	if s.Total == 0 {
		return "n/a"
	}
	return fmt.Sprintf("%d/%d", s.Covered, s.Total)
}

// Percent is the share covered, rounded down, or 0 when nothing applies.
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

// A variable so the failure below can be exercised.
var marshalIndent = json.MarshalIndent

type endpoint struct {
	SchemaVersion int    `json:"schemaVersion"`
	Label         string `json:"label"`
	Message       string `json:"message"`
	Color         string `json:"color"`
}

// Write saves the score as a shields.io endpoint badge file.
func Write(path string, s Score) error {
	data, err := marshalIndent(endpoint{
		SchemaVersion: 1,
		Label:         "zanadir",
		Message:       s.String(),
		Color:         s.colour(),
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal badge: %w", err)
	}

	// 0644: the step that publishes it may run as another user.
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil { //nolint:gosec // operator-supplied path; a badge is world-readable by design
		return fmt.Errorf("failed to write badge %s: %w", path, err)
	}
	return nil
}
