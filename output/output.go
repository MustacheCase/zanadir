package output

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/MustacheCase/zanadir/config"
	"github.com/MustacheCase/zanadir/suggester"
)

// Output interface defines the contract for output services
type Output interface {
	Response(suggestions []*suggester.CategorySuggestion, responseType string) error
}

type service struct{}

// wrapText wraps text to a specified width
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

// printTable prints suggestions in a formatted table
func printTable(suggestions []*suggester.CategorySuggestion) {
	// Print header
	fmt.Println("Category | Description | Suggested Tools")
	fmt.Println("---------|-------------|----------------")

	for _, suggestion := range suggestions {
		toolNames := []string{}
		for _, tool := range suggestion.Suggestions {
			toolNames = append(toolNames, tool.Name)
		}
		tools := strings.Join(toolNames, ", ")

		// Wrap description for better display
		description := wrapText(suggestion.Description, 60)

		// Print row
		fmt.Printf("%s | %s | %s\n", suggestion.Name, description, tools)
	}
}

// Response handles the output of suggestions in the specified format
func (s *service) Response(suggestions []*suggester.CategorySuggestion, responseType string) error {
	if responseType == config.OutputTable {
		printTable(suggestions)
		return nil
	}

	data, err := json.MarshalIndent(suggestions, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal suggestions: %w", err)
	}
	fmt.Println(string(data))
	return nil
}

// NewOutputService creates a new output service instance
func NewOutputService() Output {
	return &service{}
}
