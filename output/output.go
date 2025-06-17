package output

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/MustacheCase/zanadir/config"
	"github.com/MustacheCase/zanadir/suggester"
	"github.com/olekukonko/tablewriter"
)

// Updated interface: single Response method with a response type parameter.
type Output interface {
	Response(suggestions []*suggester.CategorySuggestion, responseType string) error
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

func (s *service) Response(suggestions []*suggester.CategorySuggestion, responseType string) error {
	if responseType == config.OutputTable {
		table := tablewriter.NewWriter(os.Stdout)
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
	fmt.Println(string(data))
	return nil
}

func NewOutputService() Output {
	return &service{}
}
