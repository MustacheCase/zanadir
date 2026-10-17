package output

import (
	"encoding/json"
	"io"
)

// jsonReport is the shape of --output json. It is a public contract, so it
// has types of its own rather than the internal ones marshalled as they are.
type jsonReport struct {
	Score         jsonScore      `json:"score"`
	Uncovered     []jsonCategory `json:"uncovered"`
	Weakened      []jsonWeakness `json:"weakened"`
	NotApplicable []string       `json:"notApplicable"`
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

type jsonWeakness struct {
	Category string `json:"category"`
	Verdict  string `json:"verdict"`
	Location string `json:"location,omitempty"`
}

func renderJSON(w io.Writer, report Report) error {
	out := jsonReport{
		Score:         jsonScore{Covered: report.Score.Covered, Total: report.Score.Total},
		Uncovered:     []jsonCategory{},
		Weakened:      []jsonWeakness{},
		NotApplicable: append([]string{}, report.NotApplicable...),
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

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(out)
}
