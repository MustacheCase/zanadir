package output

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/MustacheCase/zanadir/config"
	"github.com/MustacheCase/zanadir/score"
	"github.com/MustacheCase/zanadir/suggester"
	"github.com/olekukonko/tablewriter"
	"github.com/olekukonko/tablewriter/renderer"
	"github.com/olekukonko/tablewriter/tw"
)

// Report is one scan result to render.
type Report struct {
	Suggestions []*suggester.CategorySuggestion
	Format      string
	DestPath    string // empty means stdout
	Anchor      string // repo-relative path SARIF results point at
	// Score is the coverage summary. A zero score means none was computed and
	// the headline falls back to a plain count.
	Score score.Score
	// Weakened lists categories that have tooling which cannot fully protect
	// the repository. Informational: it never fails a scan.
	Weakened []Weakness
	// NotApplicable lists categories the repository has nothing for. They are
	// out of the score, so the report has to say which.
	NotApplicable []string
	// Platforms are the CI systems whose configuration was found.
	Platforms []string
	// Gate is what a merge into the default branch waits for; nil unless
	// --required-checks read it.
	Gate *Gate
}

// Gate is the default branch's required checks and anything wrong with them.
type Gate struct {
	Branch   string
	Required []string
	// Scanning names the code scanning tools whose alerts block a merge.
	Scanning []string
	Problems []string
}

// Weakness is a category whose tooling is present but cannot fully protect the
// repository, with the workflow the verdict came from.
type Weakness struct {
	Category string
	Verdict  string
	Location string
	// Reason explains a verdict the workflow alone does not show.
	Reason string
}

// because is a weakness's reason as a clause, or nothing.
func (w Weakness) because() string {
	if w.Reason == "" {
		return ""
	}
	return ": " + w.Reason
}

// gateLines describe what a merge into the default branch waits for.
func gateLines(g *Gate) (string, []string) {
	title := fmt.Sprintf("Merge settings for %s:", g.Branch)
	lines := []string{fmt.Sprintf("No check is required to merge into %s.", g.Branch)}
	if len(g.Required) > 0 {
		lines = []string{fmt.Sprintf("Required to merge: %s.", strings.Join(g.Required, ", "))}
	}
	if len(g.Scanning) > 0 {
		lines = append(lines, fmt.Sprintf("Code scanning alerts block a merge from: %s.", strings.Join(g.Scanning, ", ")))
	}
	for _, problem := range g.Problems {
		lines = append(lines, "Problem: "+problem+".")
	}
	return title, lines
}

// weakenedHeadline introduces the weakened block.
func weakenedHeadline(count int) string {
	if count == 1 {
		return "1 covered category is weakened:"
	}
	return fmt.Sprintf("%d covered categories are weakened:", count)
}

type Output interface {
	Response(report Report) error
}

type service struct{}

// Only the lines around the table are styled: tablewriter measures width in
// runes and escape codes would misalign every column.
const (
	ansiReset = "\x1b[0m"
	ansiBold  = "\x1b[1m"
	ansiGreen = "\x1b[32m"
)

func useColour(w io.Writer) bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func paint(enabled bool, style, text string) string {
	if !enabled {
		return text
	}
	return style + text + ansiReset
}

// headline summarises the scan, led by the coverage score when there is one.
func headline(s score.Score, count int) string {
	prefix := ""
	if s.Total > 0 {
		prefix = fmt.Sprintf("Coverage %s - ", s)
	}

	switch {
	case count == 0 && prefix == "":
		return "All categories are covered - no suggestions."
	case count == 0:
		return prefix + "all categories are covered."
	case count == 1:
		return prefix + "1 category needs attention:"
	default:
		return prefix + fmt.Sprintf("%d categories need attention:", count)
	}
}

// MarkdownMarker lets a workflow find the comment it posted last time and
// update it, instead of adding one on every push.
const MarkdownMarker = "<!-- zanadir-report -->"

// markdownCell makes a value safe to sit in a table cell: a literal pipe ends
// the cell, and a newline ends the row.
func markdownCell(text string) string {
	return strings.ReplaceAll(strings.Join(strings.Fields(text), " "), "|", "\\|")
}

func renderMarkdown(w io.Writer, suggestions []*suggester.CategorySuggestion, coverage score.Score, weaknesses []Weakness, notApplicable []string, merge *Gate) error {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n### zanadir\n\n%s\n", MarkdownMarker, headline(coverage, len(suggestions)))

	if len(suggestions) > 0 {
		b.WriteString("\n| Category | Description | Suggested tools |\n| --- | --- | --- |\n")
		for _, suggestion := range suggestions {
			tools := make([]string, 0, len(suggestion.Suggestions))
			for _, tool := range suggestion.Suggestions {
				if tool.Repository == "" {
					tools = append(tools, markdownCell(tool.Name))
					continue
				}
				tools = append(tools, fmt.Sprintf("[%s](%s)", markdownCell(tool.Name), tool.Repository))
			}
			fmt.Fprintf(&b, "| %s | %s | %s |\n",
				markdownCell(suggestion.Name), markdownCell(suggestion.Description), strings.Join(tools, ", "))
		}
	}

	if len(notApplicable) > 0 {
		fmt.Fprintf(&b, "\n_%s_\n", markdownCell(notApplicableLine(notApplicable)))
	}

	if len(weaknesses) > 0 {
		fmt.Fprintf(&b, "\n**%s**\n\n", weakenedHeadline(len(weaknesses)))
		for _, weakness := range weaknesses {
			fmt.Fprintf(&b, "- `%s` is **%s** in `%s`%s\n",
				markdownCell(weakness.Category), markdownCell(weakness.Verdict), markdownCell(weakness.Location), markdownCell(weakness.because()))
		}
	}

	if merge != nil {
		title, lines := gateLines(merge)
		fmt.Fprintf(&b, "\n**%s**\n\n", markdownCell(title))
		for _, line := range lines {
			fmt.Fprintf(&b, "- %s\n", markdownCell(line))
		}
	}

	_, err := fmt.Fprint(w, b.String())
	return err
}

func notApplicableLine(categories []string) string {
	if len(categories) == 1 {
		return fmt.Sprintf("%s is not applicable and is out of the score: nothing in this repository needs it.", categories[0])
	}
	return fmt.Sprintf("%d categories are not applicable and are out of the score: %s.",
		len(categories), strings.Join(categories, ", "))
}

func renderNotApplicable(w io.Writer, categories []string, colour bool) error {
	if len(categories) == 0 {
		return nil
	}
	_, err := fmt.Fprintf(w, "\n%s\n", paint(colour, ansiGreen, notApplicableLine(categories)))
	return err
}

// renderWeakened lists controls that are present but cannot fully protect the
// repository. It sits below the table: a missing category is still the more
// actionable finding.
func renderWeakened(w io.Writer, weaknesses []Weakness, colour bool) error {
	if len(weaknesses) == 0 {
		return nil
	}

	if _, err := fmt.Fprintf(w, "\n%s\n", paint(colour, ansiBold, weakenedHeadline(len(weaknesses)))); err != nil {
		return err
	}
	for _, weakness := range weaknesses {
		if _, err := fmt.Fprintf(w, "  %s is %s (%s)%s\n", weakness.Category, weakness.Verdict, weakness.Location, weakness.because()); err != nil {
			return err
		}
	}
	return nil
}

func renderGate(w io.Writer, merge *Gate, colour bool) error {
	if merge == nil {
		return nil
	}
	title, lines := gateLines(merge)
	if _, err := fmt.Fprintf(w, "\n%s\n", paint(colour, ansiBold, title)); err != nil {
		return err
	}
	for _, line := range lines {
		if _, err := fmt.Fprintf(w, "  %s\n", line); err != nil {
			return err
		}
	}
	return nil
}

// The returned close function is always safe to call.
func destination(path string) (io.Writer, func(), error) {
	if path == "" {
		return os.Stdout, func() {}, nil
	}
	// 0644: the action runs as root and the next step, as another user, must
	// read the report. It lists absent tooling, not secrets.
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644) //nolint:gosec // operator-supplied path; a report is world-readable by design
	if err != nil {
		return nil, func() {}, fmt.Errorf("failed to open output file %s: %w", path, err)
	}
	return f, func() { _ = f.Close() }, nil
}

func (s *service) Response(report Report) error {
	w, closeDest, err := destination(report.DestPath)
	if err != nil {
		return err
	}
	defer closeDest()

	return render(w, report)
}

func render(w io.Writer, report Report) error {
	switch report.Format {
	case config.OutputSARIF:
		return renderSarif(w, report.Suggestions, report.Anchor, report.Weakened)
	case config.OutputMarkdown:
		return renderMarkdown(w, report.Suggestions, report.Score, report.Weakened, report.NotApplicable, report.Gate)
	case config.OutputTable:
		return renderTable(w, report)
	default:
		return renderJSON(w, report)
	}
}

func renderTable(w io.Writer, report Report) error {
	suggestions := report.Suggestions
	colour := useColour(w)

	if len(suggestions) == 0 {
		if _, err := fmt.Fprintln(w, paint(colour, ansiGreen, headline(report.Score, 0))); err != nil {
			return err
		}
		return renderFooter(w, report, colour)
	}

	if _, err := fmt.Fprintf(w, "%s\n\n", paint(colour, ansiBold, headline(report.Score, len(suggestions)))); err != nil {
		return err
	}

	rows := make([][]string, 0, len(suggestions))
	for _, suggestion := range suggestions {
		toolNames := []string{}
		for _, tool := range suggestion.Suggestions {
			toolNames = append(toolNames, tool.Name)
		}
		rows = append(rows, []string{suggestion.Name, suggestion.Description, strings.Join(toolNames, ", ")})
	}

	// tablewriter drops write errors, so the table is built in memory and
	// written in one call whose error is checked.
	var buf bytes.Buffer
	table := newTable(&buf)
	table.Header("Category", "Description", "Suggested Tools")
	err := table.Bulk(rows)
	if err == nil {
		err = table.Render()
	}
	if err == nil {
		_, err = w.Write(buf.Bytes())
	}
	if err != nil {
		return err
	}
	return renderFooter(w, report, colour)
}

// renderFooter writes what follows the table: weakened controls, the merge
// settings they were judged against, and the categories left out.
func renderFooter(w io.Writer, report Report, colour bool) error {
	if err := renderWeakened(w, report.Weakened, colour); err != nil {
		return err
	}
	if err := renderGate(w, report.Gate, colour); err != nil {
		return err
	}
	return renderNotApplicable(w, report.NotApplicable, colour)
}

func NewOutputService() Output {
	return &service{}
}

// newTable draws the ASCII grid tablewriter v0 drew, so upgrading the library
// does not change what a terminal or a log file shows.
func newTable(w io.Writer) *tablewriter.Table {
	symbols := tw.NewSymbolCustom("zanadir").WithRow("-").WithColumn("|").WithCenter("|").
		WithTopLeft("|").WithTopMid("|").WithTopRight("|").
		WithMidLeft("|").WithMidRight("|").
		WithBottomLeft("|").WithBottomMid("|").WithBottomRight("|").
		WithHeaderLeft("|").WithHeaderMid("|").WithHeaderRight("|")

	return tablewriter.NewTable(w,
		tablewriter.WithRenderer(renderer.NewBlueprint(tw.Rendition{
			Symbols:  symbols,
			Settings: tw.Settings{Separators: tw.Separators{BetweenRows: tw.On}},
		})),
		tablewriter.WithRowAutoWrap(tw.WrapNormal),
		tablewriter.WithRowMaxWidth(32),
		tablewriter.WithHeaderMaxWidth(32),
	)
}
