package output

import (
	"encoding/json"
	"io"
)

// jsonReport is the shape of --output json. It is a public contract, so it
// has types of its own rather than the internal ones marshalled as they are.
type jsonReport struct {
	Score         jsonScore      `json:"score"`
	Platforms     []string       `json:"platforms"`
	Uncovered     []jsonCategory `json:"uncovered"`
	Weakened      []jsonWeakness `json:"weakened"`
	NotApplicable []string       `json:"notApplicable"`
	Covered       []jsonCoverage `json:"covered"`
	// RequiredChecks is present only when --required-checks read them.
	RequiredChecks *jsonGate `json:"requiredChecks,omitempty"`
}

type jsonGate struct {
	Branch   string   `json:"branch"`
	Required []string `json:"required"`
	Scanning []string `json:"codeScanning"`
	Problems []string `json:"problems"`
}

type jsonScore struct {
	Covered int `json:"covered"`
	Total   int `json:"total"`
}

type jsonCategory struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Suggestions []jsonTool `json:"suggestions"`
}

type jsonTool struct {
	Name        string `json:"name"`
	Repository  string `json:"repository"`
	Description string `json:"description"`
	Language    string `json:"language,omitempty"`
}

type jsonCoverage struct {
	Category string          `json:"category"`
	Verdict  string          `json:"verdict"`
	Tools    []jsonFoundTool `json:"tools"`
}

type jsonFoundTool struct {
	Name      string   `json:"name"`
	Verdict   string   `json:"verdict"`
	Locations []string `json:"locations"`
}

type jsonWeakness struct {
	Category string `json:"category"`
	Verdict  string `json:"verdict"`
	Location string `json:"location,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

func renderJSON(w io.Writer, report Report) error {
	out := jsonReport{
		Score:         jsonScore{Covered: report.Score.Covered, Total: report.Score.Total},
		Platforms:     append([]string{}, report.Platforms...),
		Uncovered:     []jsonCategory{},
		Weakened:      []jsonWeakness{},
		NotApplicable: append([]string{}, report.NotApplicable...),
		Covered:       []jsonCoverage{},
	}
	for _, category := range report.Suggestions {
		tools := []jsonTool{}
		for _, tool := range category.Suggestions {
			tools = append(tools, jsonTool{Name: tool.Name, Repository: tool.Repository, Description: tool.Description, Language: tool.Language})
		}
		out.Uncovered = append(out.Uncovered, jsonCategory{ID: category.ID, Name: category.Name, Description: category.Description, Suggestions: tools})
	}
	for _, weakness := range report.Weakened {
		out.Weakened = append(out.Weakened, jsonWeakness(weakness))
	}
	for _, c := range report.Covered {
		tools := []jsonFoundTool{}
		for _, tool := range c.Tools {
			tools = append(tools, jsonFoundTool{Name: tool.Name, Verdict: tool.Verdict, Locations: append([]string{}, tool.Locations...)})
		}
		out.Covered = append(out.Covered, jsonCoverage{Category: c.Category, Verdict: c.Verdict, Tools: tools})
	}

	if g := report.Gate; g != nil {
		out.RequiredChecks = &jsonGate{Branch: g.Branch, Required: append([]string{}, g.Required...), Scanning: append([]string{}, g.Scanning...), Problems: append([]string{}, g.Problems...)}
	}

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(out)
}
