package output

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/MustacheCase/zanadir/config"
	"github.com/MustacheCase/zanadir/suggester"
	"github.com/olekukonko/tablewriter"
)

// Output renders a scan result. destPath selects where the report goes: empty
// means stdout, otherwise the report is written to that file.
type Output interface {
	Response(suggestions []*suggester.CategorySuggestion, responseType string, destPath string) error
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

// destination resolves where a report is written. The returned close function
// is always safe to call, so callers can defer it unconditionally.
func destination(path string) (io.Writer, func(), error) {
	if path == "" {
		return os.Stdout, func() {}, nil
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600) //nolint:gosec // path is operator-supplied
	if err != nil {
		return nil, func() {}, fmt.Errorf("failed to open output file %s: %w", path, err)
	}
	return f, func() { _ = f.Close() }, nil
}

func (s *service) Response(suggestions []*suggester.CategorySuggestion, responseType string, destPath string) error {
	w, closeDest, err := destination(destPath)
	if err != nil {
		return err
	}
	defer closeDest()

	return render(w, suggestions, responseType)
}

func render(w io.Writer, suggestions []*suggester.CategorySuggestion, responseType string) error {
	if responseType == config.OutputSARIF {
		report, err := renderSarif(suggestions)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(w, report)
		return err
	}

	if responseType == config.OutputTable {
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
