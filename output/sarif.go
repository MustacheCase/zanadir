package output

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/MustacheCase/zanadir/suggester"
)

// Minimal SARIF 2.1.0 document, covering only the fields zanadir populates.
const (
	sarifVersion = "2.1.0"
	sarifSchema  = "https://json.schemastore.org/sarif-2.1.0.json"
	toolName     = "zanadir"
	toolURI      = "https://github.com/MustacheCase/zanadir"
)

type sarifLog struct {
	Schema  string     `json:"$schema"`
	Version string     `json:"version"`
	Runs    []sarifRun `json:"runs"`
}

type sarifRun struct {
	Tool    sarifTool     `json:"tool"`
	Results []sarifResult `json:"results"`
}

type sarifTool struct {
	Driver sarifDriver `json:"driver"`
}

type sarifDriver struct {
	Name           string      `json:"name"`
	InformationURI string      `json:"informationUri"`
	Rules          []sarifRule `json:"rules"`
}

type sarifRule struct {
	ID                   string            `json:"id"`
	Name                 string            `json:"name"`
	ShortDescription     sarifMessage      `json:"shortDescription"`
	FullDescription      sarifMessage      `json:"fullDescription"`
	Help                 sarifMessage      `json:"help"`
	DefaultConfiguration sarifRuleConfig   `json:"defaultConfiguration"`
	Properties           sarifRuleProperty `json:"properties"`
}

type sarifRuleConfig struct {
	Level string `json:"level"`
}

type sarifRuleProperty struct {
	Tags []string `json:"tags"`
}

type sarifMessage struct {
	Text string `json:"text"`
}

type sarifResult struct {
	RuleID              string            `json:"ruleId"`
	Level               string            `json:"level"`
	Message             sarifMessage      `json:"message"`
	PartialFingerprints map[string]string `json:"partialFingerprints"`
}

// helpText renders a category's suggested tools as a list.
func helpText(suggestion *suggester.CategorySuggestion) string {
	var b strings.Builder
	b.WriteString(suggestion.Description)
	if len(suggestion.Suggestions) == 0 {
		return b.String()
	}
	b.WriteString("\n\nSuggested tools:\n")
	for _, tool := range suggestion.Suggestions {
		fmt.Fprintf(&b, "- %s (%s): %s\n", tool.Name, tool.Repository, tool.Description)
	}
	return strings.TrimRight(b.String(), "\n")
}

// buildSarif converts category suggestions into a SARIF log: one rule and one
// result per uncovered category. Results carry no physical location, since a
// missing control is the absence of configuration rather than a defect in a file.
func buildSarif(suggestions []*suggester.CategorySuggestion) sarifLog {
	rules := make([]sarifRule, 0, len(suggestions))
	results := make([]sarifResult, 0, len(suggestions))

	for _, suggestion := range suggestions {
		toolNames := make([]string, 0, len(suggestion.Suggestions))
		for _, tool := range suggestion.Suggestions {
			toolNames = append(toolNames, tool.Name)
		}

		rules = append(rules, sarifRule{
			ID:                   suggestion.ID,
			Name:                 suggestion.Name,
			ShortDescription:     sarifMessage{Text: fmt.Sprintf("Missing CI/CD coverage: %s", suggestion.Name)},
			FullDescription:      sarifMessage{Text: suggestion.Description},
			Help:                 sarifMessage{Text: helpText(suggestion)},
			DefaultConfiguration: sarifRuleConfig{Level: "warning"},
			Properties:           sarifRuleProperty{Tags: []string{"ci-cd", "coverage"}},
		})

		message := fmt.Sprintf("No %s tooling detected in this repository's CI configuration.", suggestion.Name)
		if len(toolNames) > 0 {
			message = fmt.Sprintf("%s Consider adding one of: %s.", message, strings.Join(toolNames, ", "))
		}

		results = append(results, sarifResult{
			RuleID:  suggestion.ID,
			Level:   "warning",
			Message: sarifMessage{Text: message},
			// The category is the finding's whole identity.
			PartialFingerprints: map[string]string{"categoryId": suggestion.ID},
		})
	}

	return sarifLog{
		Schema:  sarifSchema,
		Version: sarifVersion,
		Runs: []sarifRun{{
			Tool: sarifTool{Driver: sarifDriver{
				Name:           toolName,
				InformationURI: toolURI,
				Rules:          rules,
			}},
			Results: results,
		}},
	}
}

func renderSarif(suggestions []*suggester.CategorySuggestion) (string, error) {
	data, err := json.MarshalIndent(buildSarif(suggestions), "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal SARIF report: %w", err)
	}
	return string(data), nil
}
