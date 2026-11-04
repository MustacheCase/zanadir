package skills

import (
	"cmp"
	"fmt"
	"io"
	"strings"

	"github.com/MustacheCase/zanadir/internal/markdown"
	"github.com/MustacheCase/zanadir/internal/sarif"
)

// MarkdownMarker lets a workflow find the comment it posted last time.
const MarkdownMarker = "<!-- zanadir-skills-report -->"

// maxMarkdownFindings keeps a comment well under GitHub's size limit.
const maxMarkdownFindings = 50

type ruleInfo struct{ id, finds, level string }

var ruleInfos = []ruleInfo{
	{"frontmatter-missing", "a SKILL.md with no frontmatter", ""},
	{"frontmatter-invalid", "frontmatter that is not valid YAML", Error},
	{"name-missing", "frontmatter with no name", ""},
	{"name-format", "a name that is not lowercase letters, digits and single hyphens", ""},
	{"name-mismatch", "a name that does not match the skill's directory", ""},
	{"description-missing", "frontmatter with no description", ""},
	{"description-short", "a description under 40 characters, too little for the agent to know when to use the skill", ""},
	{"description-long", "a description over the 1,024 characters the Agent Skills specification allows", ""},
	{"description-no-trigger", "a description that says what the skill does but never when to use it", ""},
	{"description-overlap", "two skills in one skills directory whose descriptions share most of their words", ""},
	{"body-long", "a SKILL.md over 500 lines, which loads whole every time", ""},
	{"bash-unrestricted", "allowed-tools granting every shell command", ""},
	{"missing-reference", "a link to a file in the repository that does not exist", Error},
	{"hidden-unicode", "invisible characters that can hide text from a reviewer", Error},
	{"pipe-to-shell", "a script downloaded and piped straight into a shell", ""},
	{"skip-approvals", "running the agent without approvals or outside its sandbox", ""},
}

// WriteSARIF writes the findings for code scanning, under a category of
// their own so they do not replace a scan's results.
func (r Report) WriteSARIF(w io.Writer) error {
	rules := make([]sarif.Rule, 0, len(ruleInfos))
	for _, info := range ruleInfos {
		rules = append(rules, sarif.Rule{
			ID:                   "skills/" + info.id,
			Name:                 info.id,
			ShortDescription:     sarif.Message{Text: "Agent skill: " + info.finds},
			FullDescription:      sarif.Message{Text: "zanadir skills found " + info.finds + "."},
			Help:                 sarif.Message{Text: "See https://github.com/MustacheCase/zanadir#zanadir-skills for every rule."},
			DefaultConfiguration: sarif.RuleConfig{Level: cmp.Or(info.level, Warning)},
			Properties:           sarif.RuleProperty{Tags: []string{"agent-skills"}},
		})
	}
	results := make([]sarif.Result, 0, len(r.Findings))
	for _, f := range r.Findings {
		results = append(results, sarif.Result{
			RuleID:    "skills/" + f.Rule,
			Level:     f.Severity,
			Message:   sarif.Message{Text: f.Message},
			Locations: sarif.At(f.File, f.Line),
		})
	}
	return sarif.Write(w, sarif.Log{
		Schema:  sarif.Schema,
		Version: sarif.Version,
		Runs: []sarif.Run{{
			Tool:              sarif.Tool{Driver: sarif.Driver{Name: "zanadir", InformationURI: "https://github.com/MustacheCase/zanadir", Rules: rules}},
			AutomationDetails: &sarif.AutomationDetails{ID: "zanadir/skills/"},
			Results:           results,
		}},
	})
}

// WriteMarkdown writes the report as a pull request comment, errors first.
func (r Report) WriteMarkdown(w io.Writer) error {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n### zanadir skills\n\n", MarkdownMarker)
	switch {
	case len(r.Skills) == 0:
		b.WriteString("No skills found (no SKILL.md files).\n")
	case len(r.Findings) == 0:
		fmt.Fprintf(&b, "Checked %d %s: no problems found.\n", len(r.Skills), plural(len(r.Skills), "skill"))
	default:
		warnings := len(r.Findings) - r.Errors()
		fmt.Fprintf(&b, "Checked %d %s: %d %s, %d %s.\n\n| Severity | Problem | Where | Rule |\n| --- | --- | --- | --- |\n",
			len(r.Skills), plural(len(r.Skills), "skill"), r.Errors(), plural(r.Errors(), "error"), warnings, plural(warnings, "warning"))
		shown := 0
		for _, severity := range []string{Error, Warning} {
			for _, f := range r.Findings {
				if f.Severity != severity || shown == maxMarkdownFindings {
					continue
				}
				shown++
				fmt.Fprintf(&b, "| %s | %s | %s | %s |\n", severity, markdown.Cell(markdown.Text(f.Message)),
					markdown.Cell(markdown.Code(fmt.Sprintf("%s:%d", f.File, f.Line))), markdown.Code(f.Rule))
			}
		}
		if rest := len(r.Findings) - shown; rest > 0 {
			fmt.Fprintf(&b, "\n_%d more not shown; run `zanadir skills` to see them all._\n", rest)
		}
	}
	_, err := io.WriteString(w, b.String())
	return err
}
