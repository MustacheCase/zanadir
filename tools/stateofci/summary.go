package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/MustacheCase/zanadir/models"
)

// summary holds aggregates only: nothing in it names a repository.
type summary struct {
	Date           string         `json:"date"`
	Repositories   int            `json:"repositories"`
	Scanned        int            `json:"scanned"`
	WithoutCI      int            `json:"withoutCI"`
	Failed         int            `json:"failed"`
	Platforms      map[string]int `json:"platforms"`
	MedianCoverage float64        `json:"medianCoverage"`
	WithWeakened   int            `json:"withWeakened"`
	Categories     []categoryStat `json:"categories"`
}

type categoryStat struct {
	Category   string `json:"category"`
	Applicable int    `json:"applicable"`
	Covered    int    `json:"covered"`
	// Weakened counts pipeline controls only; BotOnly counts categories whose
	// strongest control is a dependency bot, which never fails a build by
	// design and would otherwise swamp the SCA figure.
	Weakened int `json:"weakened"`
	Advisory int `json:"advisory"`
	Partial  int `json:"partial"`
	BotOnly  int `json:"botOnly"`
}

// aggregate summarises the first n results with CI, in candidate order.
func aggregate(dir string, candidates []candidate, n int, now time.Time) summary {
	s := summary{Date: now.Format("2006-01-02"), Platforms: map[string]int{}}
	stats := map[string]*categoryStat{}
	for _, title := range models.CategoryTitles {
		stats[string(title)] = &categoryStat{Category: string(title)}
	}

	var coverage []float64
	for _, c := range candidates {
		if s.Repositories == n {
			break
		}
		r, ok := readResult(dir, c.FullName)
		switch {
		case !ok && exists(rawPath(dir, c.FullName)+".err"):
			s.Failed++
			continue
		case !ok:
			continue
		}
		s.Scanned++
		if len(r.Platforms) == 0 {
			s.WithoutCI++
			continue
		}

		s.Repositories++
		for _, p := range r.Platforms {
			s.Platforms[p]++
		}
		if r.Score.Total > 0 {
			coverage = append(coverage, float64(r.Score.Covered)/float64(r.Score.Total))
		}
		for _, w := range r.Weakened {
			if !isBotConfig(w.Location) {
				s.WithWeakened++
				break
			}
		}
		addCategories(stats, r)
	}

	s.MedianCoverage = median(coverage)
	for _, title := range models.CategoryTitles {
		s.Categories = append(s.Categories, *stats[string(title)])
	}
	return s
}

func addCategories(stats map[string]*categoryStat, r result) {
	skip := map[string]bool{}
	for _, c := range r.NotApplicable {
		skip[c] = true
	}
	for _, u := range r.Uncovered {
		skip[u.ID] = true
	}
	for category, stat := range stats {
		if !slices.Contains(r.NotApplicable, category) {
			stat.Applicable++
		}
		if !skip[category] {
			stat.Covered++
		}
	}
	for _, w := range r.Weakened {
		stat, ok := stats[w.Category]
		switch {
		case !ok:
		case isBotConfig(w.Location):
			stat.BotOnly++
		default:
			stat.Weakened++
			if w.Verdict == "advisory" {
				stat.Advisory++
			} else {
				stat.Partial++
			}
		}
	}
}

// isBotConfig reports whether a weakened control's evidence is a dependency
// bot's configuration rather than a pipeline.
func isBotConfig(location string) bool {
	base := strings.ToLower(filepath.Base(location))
	return strings.HasPrefix(base, "dependabot.") || strings.HasPrefix(base, "renovate") || strings.HasPrefix(base, ".renovaterc")
}

func median(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := slices.Sorted(slices.Values(values))
	mid := len(sorted) / 2
	if len(sorted)%2 == 1 {
		return sorted[mid]
	}
	return (sorted[mid-1] + sorted[mid]) / 2
}

func percent(part, whole int) string {
	if whole == 0 {
		return "-"
	}
	return fmt.Sprintf("%.0f%%", 100*float64(part)/float64(whole))
}

// writeMarkdown renders the summary for people; summary.json is for tools.
func writeMarkdown(w io.Writer, s summary) error {
	var b strings.Builder
	fmt.Fprintf(&b, "# State of CI security - %s\n\n", s.Date)
	fmt.Fprintf(&b, "%d of the most-starred GitHub repositories with CI, scanned with zanadir. ", s.Repositories)
	fmt.Fprintf(&b, "%d more were scanned and had no CI; %d could not be scanned.\n\n", s.WithoutCI, s.Failed)
	fmt.Fprintf(&b, "- Median coverage: %.0f%% of the categories that apply\n", 100*s.MedianCoverage)
	fmt.Fprintf(&b, "- Repositories with at least one control that cannot fail the build: %s\n\n", percent(s.WithWeakened, s.Repositories))

	b.WriteString("| Category | Covered (where it applies) | Of those, cannot fail the build | advisory | partial | Only a dependency bot |\n|---|---|---|---|---|---|\n")
	for _, c := range s.Categories {
		fmt.Fprintf(&b, "| %s | %s | %s | %d | %d | %s |\n", c.Category, percent(c.Covered, c.Applicable),
			percent(c.Weakened, c.Covered), c.Advisory, c.Partial, percent(c.BotOnly, c.Covered))
	}
	b.WriteString("\n\"Cannot fail the build\" counts pipeline tools only. Dependabot and Renovate open pull requests rather than fail builds, so a category whose only control is one of them is counted apart.\n")

	b.WriteString("\n| Platform | Repositories |\n|---|---|\n")
	platforms := make([]string, 0, len(s.Platforms))
	for p := range s.Platforms {
		platforms = append(platforms, p)
	}
	slices.SortFunc(platforms, func(a, b string) int { return s.Platforms[b] - s.Platforms[a] })
	for _, p := range platforms {
		fmt.Fprintf(&b, "| %s | %d |\n", p, s.Platforms[p])
	}
	_, err := io.WriteString(w, b.String())
	return err
}

func writeSummary(dir string, candidates []candidate, n int, out string) error {
	s := aggregate(dir, candidates, n, time.Now())
	if err := writeJSON(filepath.Join(out, "summary.json"), s); err != nil {
		return err
	}
	f, err := os.Create(filepath.Join(out, "summary.md"))
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return writeMarkdown(f, s)
}
