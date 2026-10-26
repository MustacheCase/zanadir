package skills

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/MustacheCase/zanadir/utils"
)

// Report is the result of checking a directory's skills.
type Report struct {
	Skills   []string  `json:"skills"`
	Findings []Finding `json:"findings"`
}

// Run finds and checks the skills under dir.
func Run(dir string) (Report, error) {
	found, err := Discover(dir)
	if err != nil {
		return Report{}, err
	}
	findings := Check(dir, found)
	if findings == nil {
		findings = []Finding{}
	}
	return Report{Skills: Names(found), Findings: findings}, nil
}

// Errors counts the findings of error severity.
func (r Report) Errors() int {
	n := 0
	for _, f := range r.Findings {
		if f.Severity == Error {
			n++
		}
	}
	return n
}

// WriteJSON writes the report as indented JSON.
func (r Report) WriteJSON(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}

// WriteTable writes the report for a terminal, grouped by category.
func (r Report) WriteTable(w io.Writer) error {
	var b strings.Builder
	if len(r.Skills) == 0 {
		b.WriteString("No skills found (no SKILL.md files).\n")
		_, err := io.WriteString(w, b.String())
		return err
	}
	fmt.Fprintf(&b, "Found %d %s: %s\n", len(r.Skills), plural(len(r.Skills), "skill"), utils.Printable(strings.Join(r.Skills, ", ")))
	for _, category := range []string{Quality, Safety} {
		var lines []string
		for _, f := range r.Findings {
			if f.Category == category {
				mark := "!"
				if f.Severity == Error {
					mark = "✗"
				}
				lines = append(lines, fmt.Sprintf("  %s %s:%d  %s  [%s]\n", mark, utils.Printable(f.File), f.Line, utils.Printable(f.Message), f.Rule))
			}
		}
		if len(lines) > 0 {
			fmt.Fprintf(&b, "\n%s\n%s", category, strings.Join(lines, ""))
		}
	}
	warnings := len(r.Findings) - r.Errors()
	if len(r.Findings) == 0 {
		b.WriteString("\nNo problems found.\n")
	} else {
		fmt.Fprintf(&b, "\n%d %s, %d %s\n", r.Errors(), plural(r.Errors(), "error"), warnings, plural(warnings, "warning"))
	}
	_, err := io.WriteString(w, b.String())
	return err
}

func plural(n int, word string) string {
	if n == 1 {
		return word
	}
	return word + "s"
}
