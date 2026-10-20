package output

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/MustacheCase/zanadir/models"
)

// OrgReport is an organisation-wide scan: one report per repository scanned,
// and the repositories skipped or that could not be scanned.
type OrgReport struct {
	Org          string
	Repositories []OrgRepository
	Skipped      []OrgRepository
	Failed       []OrgRepository
	// Interrupted means a signal stopped the scan before every repository
	// was reached.
	Interrupted bool
	Format      string
	DestPath    string
}

// OrgRepository is one repository: its report, or why it has none.
type OrgRepository struct {
	Name   string
	Report Report
	Reason string
}

// maxTableRepositories bounds the repository table; JSON has them all.
const maxTableRepositories = 50

// WriteOrg writes an organisation report as a table or JSON.
func WriteOrg(report OrgReport) error {
	w, closeDest, err := destination(report.DestPath)
	if err != nil {
		return err
	}
	defer closeDest()
	if report.Format == "json" {
		return writeJSON(w, orgJSONOf(report))
	}
	return renderOrgTable(w, report)
}

// worstFirst orders repositories by the share of categories covered, then
// by how many controls are weakened, then by name.
func worstFirst(repositories []OrgRepository) []OrgRepository {
	sorted := slices.Clone(repositories)
	share := func(r OrgRepository) float64 {
		if r.Report.Score.Total == 0 {
			return 1
		}
		return float64(r.Report.Score.Covered) / float64(r.Report.Score.Total)
	}
	slices.SortStableFunc(sorted, func(a, b OrgRepository) int {
		return cmp.Or(cmp.Compare(share(a), share(b)), cmp.Compare(len(b.Report.Weakened), len(a.Report.Weakened)),
			strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)))
	})
	return sorted
}

type orgCategory struct {
	name                          string
	uncovered, weakened, ruledOut int
	uncoveredIn                   []string
}

// orgCategories counts, for each category, the repositories where it is
// uncovered, weakened or not applicable.
func orgCategories(repositories []OrgRepository) []orgCategory {
	var categories []orgCategory
	for _, title := range models.CategoryTitles {
		c := orgCategory{name: string(title)}
		for _, r := range repositories {
			for _, s := range r.Report.Suggestions {
				if s.ID == string(title) {
					c.uncovered++
					c.uncoveredIn = append(c.uncoveredIn, r.Name)
				}
			}
			for _, w := range r.Report.Weakened {
				if w.Category == string(title) {
					c.weakened++
				}
			}
			if slices.Contains(r.Report.NotApplicable, string(title)) {
				c.ruledOut++
			}
		}
		categories = append(categories, c)
	}
	return categories
}

func orgHeadline(report OrgReport) string {
	line := fmt.Sprintf("%s: %d repositories scanned", report.Org, len(report.Repositories))
	if len(report.Skipped) > 0 {
		line += fmt.Sprintf(", %d skipped", len(report.Skipped))
	}
	if len(report.Failed) > 0 {
		line += fmt.Sprintf(", %d could not be scanned", len(report.Failed))
	}
	if report.Interrupted {
		line += "; interrupted, so this covers only the repositories reached"
	}
	return line
}

func renderOrgTable(w io.Writer, report OrgReport) error {
	var b bytes.Buffer
	var tableErr error
	fmt.Fprintf(&b, "%s\n", orgHeadline(report))

	repositories := worstFirst(report.Repositories)
	if len(repositories) > 0 {
		rows := [][]string{}
		for i, r := range repositories {
			if i == maxTableRepositories {
				break
			}
			uncovered := make([]string, 0, len(r.Report.Suggestions))
			for _, s := range r.Report.Suggestions {
				uncovered = append(uncovered, s.ID)
			}
			uncovered = atMost(uncovered, 4)
			if len(r.Report.Platforms) == 0 {
				uncovered = []string{"no CI found"}
			}
			weakened := make([]string, 0, len(r.Report.Weakened))
			for _, wk := range r.Report.Weakened {
				weakened = append(weakened, fmt.Sprintf("%s (%s)", wk.Category, wk.Verdict))
			}
			rows = append(rows, []string{short(report.Org, r.Name), r.Report.Score.String(), strings.Join(uncovered, ", "), strings.Join(weakened, ", ")})
		}
		b.WriteString("\n")
		tableErr = writeTable(&b, []string{"Repository", "Score", "Uncovered", "Weakened"}, rows)
		if len(repositories) > maxTableRepositories {
			fmt.Fprintf(&b, "The %d worst of %d are shown; -o json lists every repository.\n", maxTableRepositories, len(repositories))
		}

		rows = [][]string{}
		for _, c := range orgCategories(repositories) {
			if c.uncovered == 0 && c.weakened == 0 {
				continue
			}
			named := make([]string, 0, len(c.uncoveredIn))
			for _, name := range c.uncoveredIn {
				named = append(named, short(report.Org, name))
			}
			named = atMost(named, 5)
			rows = append(rows, []string{c.name, fmt.Sprintf("%d of %d", c.uncovered, len(repositories)-c.ruledOut), fmt.Sprint(c.weakened), strings.Join(named, ", ")})
		}
		if len(rows) > 0 {
			b.WriteString("\n")
			tableErr = errors.Join(tableErr, writeTable(&b, []string{"Category", "Uncovered in", "Weakened in", "Uncovered repositories"}, rows))
		}
	}

	for _, group := range []struct {
		title        string
		repositories []OrgRepository
	}{{"Could not be scanned", report.Failed}, {"Skipped", report.Skipped}} {
		if len(group.repositories) == 0 {
			continue
		}
		fmt.Fprintf(&b, "\n%s:\n", group.title)
		for _, r := range group.repositories {
			fmt.Fprintf(&b, "  %s: %s\n", short(report.Org, r.Name), r.Reason)
		}
	}
	_, err := w.Write(b.Bytes())
	return errors.Join(tableErr, err)
}

// short drops the organisation, which the headline names, from a repository.
func short(org, name string) string {
	return strings.TrimPrefix(name, org+"/")
}

// atMost keeps n names and counts the rest.
func atMost(names []string, n int) []string {
	if len(names) <= n {
		return names
	}
	return append(slices.Clone(names[:n]), fmt.Sprintf("and %d more", len(names)-n))
}

func writeTable(w io.Writer, header []string, rows [][]string) error {
	table := newTable(w)
	cells := make([]any, len(header))
	for i, h := range header {
		cells[i] = h
	}
	table.Header(cells...)
	err := table.Bulk(rows)
	if err == nil {
		err = table.Render()
	}
	return err
}

type orgJSON struct {
	Organisation string           `json:"organisation"`
	Scanned      int              `json:"scanned"`
	Interrupted  bool             `json:"interrupted,omitempty"`
	Repositories []orgJSONEntry   `json:"repositories"`
	Categories   []orgJSONSummary `json:"categories"`
	Skipped      []orgJSONReason  `json:"skipped"`
	Failed       []orgJSONReason  `json:"failed"`
}

type orgJSONEntry struct {
	Repository string     `json:"repository"`
	Report     jsonReport `json:"report"`
}

type orgJSONSummary struct {
	Category      string `json:"category"`
	Uncovered     int    `json:"uncovered"`
	Weakened      int    `json:"weakened"`
	NotApplicable int    `json:"notApplicable"`
}

type orgJSONReason struct {
	Repository string `json:"repository"`
	Reason     string `json:"reason"`
}

func orgJSONOf(report OrgReport) orgJSON {
	out := orgJSON{Organisation: report.Org, Scanned: len(report.Repositories), Interrupted: report.Interrupted,
		Repositories: []orgJSONEntry{}, Categories: []orgJSONSummary{}, Skipped: []orgJSONReason{}, Failed: []orgJSONReason{}}
	for _, r := range worstFirst(report.Repositories) {
		out.Repositories = append(out.Repositories, orgJSONEntry{Repository: r.Name, Report: jsonOf(r.Report)})
	}
	for _, c := range orgCategories(report.Repositories) {
		out.Categories = append(out.Categories, orgJSONSummary{Category: c.name, Uncovered: c.uncovered, Weakened: c.weakened, NotApplicable: c.ruledOut})
	}
	for _, r := range report.Skipped {
		out.Skipped = append(out.Skipped, orgJSONReason{Repository: r.Name, Reason: r.Reason})
	}
	for _, r := range report.Failed {
		out.Failed = append(out.Failed, orgJSONReason{Repository: r.Name, Reason: r.Reason})
	}
	return out
}
