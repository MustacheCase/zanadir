package output

import (
	"cmp"
	"fmt"
	"io"
	"strings"

	"github.com/MustacheCase/zanadir/grading"
	"github.com/MustacheCase/zanadir/internal/sarif"
	"github.com/MustacheCase/zanadir/suggester"
)

const (
	toolName = "zanadir"
	toolURI  = "https://github.com/MustacheCase/zanadir"
)

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

// weakenedRuleID keeps a weakened control distinct from a missing one, so code
// scanning tracks them as separate alerts rather than one changing its mind.
func weakenedRuleID(category string) string {
	return category + "/weakened"
}

// buildSarif converts suggestions into one rule and one result per category.
func buildSarif(suggestions []*suggester.CategorySuggestion, anchor string, weaknesses []Weakness) sarif.Log {
	rules := make([]sarif.Rule, 0, len(suggestions))
	results := make([]sarif.Result, 0, len(suggestions))

	for _, suggestion := range suggestions {
		toolNames := make([]string, 0, len(suggestion.Suggestions))
		for _, tool := range suggestion.Suggestions {
			toolNames = append(toolNames, tool.Name)
		}

		rules = append(rules, sarif.Rule{
			ID:                   suggestion.ID,
			Name:                 suggestion.Name,
			ShortDescription:     sarif.Message{Text: fmt.Sprintf("Missing CI/CD coverage: %s", suggestion.Name)},
			FullDescription:      sarif.Message{Text: suggestion.Description},
			Help:                 sarif.Message{Text: helpText(suggestion)},
			DefaultConfiguration: sarif.RuleConfig{Level: "warning"},
			Properties:           sarif.RuleProperty{Tags: []string{"ci-cd", "coverage"}},
		})

		message := fmt.Sprintf("No %s tooling detected in this repository's CI configuration.", suggestion.Name)
		if len(toolNames) > 0 {
			message = fmt.Sprintf("%s Consider adding one of: %s.", message, strings.Join(toolNames, ", "))
		}

		results = append(results, sarif.Result{
			RuleID:              suggestion.ID,
			Level:               "warning",
			Message:             sarif.Message{Text: message},
			PartialFingerprints: map[string]string{"categoryId": suggestion.ID},
			Locations:           sarif.At(anchor, 1),
		})
	}

	// A weakened control is a note: the tooling is there, so it is a weaker
	// finding than a category with nothing at all, and this ships
	// informational before it can fail anything.
	for _, weakness := range weaknesses {
		ruleID := weakenedRuleID(weakness.Category)

		rules = append(rules, sarif.Rule{
			ID:                   ruleID,
			Name:                 weakness.Category,
			ShortDescription:     sarif.Message{Text: fmt.Sprintf("Weakened CI/CD control: %s", weakness.Category)},
			FullDescription:      sarif.Message{Text: fmt.Sprintf("%s tooling is present but cannot fully protect this repository.", weakness.Category)},
			Help:                 sarif.Message{Text: weakenedHelp(weakness)},
			DefaultConfiguration: sarif.RuleConfig{Level: "note"},
			Properties:           sarif.RuleProperty{Tags: []string{"ci-cd", "coverage", "effectiveness"}},
		})

		results = append(results, sarif.Result{
			RuleID:  ruleID,
			Level:   "note",
			Message: sarif.Message{Text: weakenedMessage(weakness)},
			PartialFingerprints: map[string]string{
				"categoryId": weakness.Category,
				"verdict":    weakness.Verdict,
			},
			// The workflow that produced the verdict, which is a better
			// location than the anchor, unless no file did (default setup).
			Locations: sarif.At(cmp.Or(weakness.Location, anchor), 1),
		})
	}

	return sarif.Log{
		Schema:  sarif.Schema,
		Version: sarif.Version,
		Runs: []sarif.Run{{
			Tool: sarif.Tool{Driver: sarif.Driver{
				Name:           toolName,
				InformationURI: toolURI,
				Rules:          rules,
			}},
			Results: results,
		}},
	}
}

// weakenedHelp explains a verdict in terms of what it fails to stop.
func weakenedHelp(w Weakness) string {
	switch {
	case w.Verdict == "advisory":
		return "it runs but cannot fail the build, so nothing it finds blocks a change."
	case w.Verdict == "partial" && (w.Reason == "" || w.Reason == grading.NotRequired):
		return "it can fail a build, but not on the path that gates a change, so pull requests go unchecked."
	case w.Verdict == "partial":
		return "it can fail a build, but cannot see part of what it is meant to check."
	default:
		return "it provides less protection than its presence suggests."
	}
}

func weakenedMessage(w Weakness) string {
	switch w.Reason {
	case "":
		return fmt.Sprintf("%s tooling is present but %s: %s", w.Category, w.Verdict, weakenedHelp(w))
	case grading.NotRequired:
		return fmt.Sprintf("%s tooling is present but %s: %s, so a failing run does not block a pull request.", w.Category, w.Verdict, w.Reason)
	default:
		return fmt.Sprintf("%s tooling is present but %s: %s.", w.Category, w.Verdict, w.Reason)
	}
}

func renderSarif(w io.Writer, suggestions []*suggester.CategorySuggestion, anchor string, weaknesses []Weakness) error {
	return sarif.Write(w, buildSarif(suggestions, anchor, weaknesses))
}
