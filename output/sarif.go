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
	Locations           []sarifLocation   `json:"locations,omitempty"`
	PartialFingerprints map[string]string `json:"partialFingerprints"`
}

type sarifLocation struct {
	PhysicalLocation sarifPhysicalLocation `json:"physicalLocation"`
}

type sarifPhysicalLocation struct {
	ArtifactLocation sarifArtifactLocation `json:"artifactLocation"`
	Region           sarifRegion           `json:"region"`
}

type sarifArtifactLocation struct {
	URI string `json:"uri"`
}

type sarifRegion struct {
	StartLine int `json:"startLine"`
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

// locateAt builds the single-location list SARIF wants, or none when there is
// nowhere to point. Code scanning rejects a result with no location.
func locateAt(uri string) []sarifLocation {
	if uri == "" {
		return nil
	}
	return []sarifLocation{{
		PhysicalLocation: sarifPhysicalLocation{
			ArtifactLocation: sarifArtifactLocation{URI: uri},
			Region:           sarifRegion{StartLine: 1},
		},
	}}
}

// weakenedRuleID keeps a weakened control distinct from a missing one, so code
// scanning tracks them as separate alerts rather than one changing its mind.
func weakenedRuleID(category string) string {
	return category + "/weakened"
}

// buildSarif converts suggestions into one rule and one result per category.
func buildSarif(suggestions []*suggester.CategorySuggestion, anchor string, weaknesses []Weakness) sarifLog {
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
			RuleID:              suggestion.ID,
			Level:               "warning",
			Message:             sarifMessage{Text: message},
			PartialFingerprints: map[string]string{"categoryId": suggestion.ID},
			Locations:           locateAt(anchor),
		})
	}

	// A weakened control is a note: the tooling is there, so it is a weaker
	// finding than a category with nothing at all, and this ships
	// informational before it can fail anything.
	for _, weakness := range weaknesses {
		ruleID := weakenedRuleID(weakness.Category)

		rules = append(rules, sarifRule{
			ID:                   ruleID,
			Name:                 weakness.Category,
			ShortDescription:     sarifMessage{Text: fmt.Sprintf("Weakened CI/CD control: %s", weakness.Category)},
			FullDescription:      sarifMessage{Text: fmt.Sprintf("%s tooling is present but cannot fully protect this repository.", weakness.Category)},
			Help:                 sarifMessage{Text: weakenedHelp(weakness.Verdict)},
			DefaultConfiguration: sarifRuleConfig{Level: "note"},
			Properties:           sarifRuleProperty{Tags: []string{"ci-cd", "coverage", "effectiveness"}},
		})

		results = append(results, sarifResult{
			RuleID:  ruleID,
			Level:   "note",
			Message: sarifMessage{Text: fmt.Sprintf("%s tooling is present but %s: %s", weakness.Category, weakness.Verdict, weakenedHelp(weakness.Verdict))},
			PartialFingerprints: map[string]string{
				"categoryId": weakness.Category,
				"verdict":    weakness.Verdict,
			},
			// The workflow that produced the verdict, which is a better
			// location than the anchor a missing category has to fall back on.
			Locations: locateAt(weakness.Location),
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

// weakenedHelp explains a verdict in terms of what it fails to stop.
func weakenedHelp(verdict string) string {
	switch verdict {
	case "advisory":
		return "it runs but cannot fail the build, so nothing it finds blocks a change."
	case "partial":
		return "it can fail a build, but not on the path that gates a change, so pull requests go unchecked."
	default:
		return "it provides less protection than its presence suggests."
	}
}

func renderSarif(suggestions []*suggester.CategorySuggestion, anchor string, weaknesses []Weakness) (string, error) {
	data, err := json.MarshalIndent(buildSarif(suggestions, anchor, weaknesses), "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal SARIF report: %w", err)
	}
	return string(data), nil
}
