package output

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/MustacheCase/zanadir/config"
	"github.com/MustacheCase/zanadir/score"
	"github.com/MustacheCase/zanadir/suggester"
	"github.com/olekukonko/tablewriter"
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
}

type Output interface {
	Response(report Report) error
}

type service struct{}

func wrapText(text string, lineWidth int) string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return ""
	}

	var lines []string
	var currentLine string

	for _, word := range words {
		if len(currentLine)+len(word)+1 > lineWidth {
			lines = append(lines, currentLine)
			currentLine = word
		} else {
			if currentLine != "" {
				currentLine += " "
			}
			currentLine += word
		}
	}
	if currentLine != "" {
		lines = append(lines, currentLine)
	}
	return strings.Join(lines, "\n")
}

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
	suggestions, responseType := report.Suggestions, report.Format
	if responseType == config.OutputSARIF {
		sarifReport, err := renderSarif(suggestions, report.Anchor)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(w, sarifReport)
		return err
	}

	if responseType == config.OutputTable {
		colour := useColour(w)

		if len(suggestions) == 0 {
			_, err := fmt.Fprintln(w, paint(colour, ansiGreen, headline(report.Score, 0)))
			return err
		}

		if _, err := fmt.Fprintf(w, "%s\n\n", paint(colour, ansiBold, headline(report.Score, len(suggestions)))); err != nil {
			return err
		}

		table := tablewriter.NewWriter(w)
		table.SetHeader([]string{"Category", "Description", "Suggested Tools"})
		table.SetBorders(tablewriter.Border{Left: true, Top: true, Right: true, Bottom: true})
		table.SetCenterSeparator("|")
		table.SetColumnSeparator("|")
		table.SetRowSeparator("-")
		table.SetRowLine(true)
		table.SetAutoWrapText(true)
		table.SetReflowDuringAutoWrap(true)

		for _, suggestion := range suggestions {
			toolNames := []string{}
			for _, tool := range suggestion.Suggestions {
				toolNames = append(toolNames, tool.Name)
			}
			tools := strings.Join(toolNames, ", ")

			// Wrap description for better display
			description := wrapText(suggestion.Description, 60)

			table.Append([]string{suggestion.Name, description, tools})
		}

		table.Render()
		return nil
	}

	data, err := json.MarshalIndent(suggestions, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal suggestions: %w", err)
	}
	_, err = fmt.Fprintln(w, string(data))
	return err
}

func NewOutputService() Output {
	return &service{}
}
