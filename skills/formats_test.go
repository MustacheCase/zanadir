package skills

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/MustacheCase/zanadir/internal/sarif"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A rule added to the checks without a SARIF entry would reach code
// scanning with no description.
func TestEveryRuleHasSARIFInfo(t *testing.T) {
	source, err := os.ReadFile("rules.go")
	require.NoError(t, err)
	known := map[string]bool{}
	for _, info := range ruleInfos {
		known[info.id] = true
	}
	found := regexp.MustCompile(`(?:quality|safety)\([a-z], "([a-z-]+)"|rule: +"([a-z-]+)"`).FindAllStringSubmatch(string(source), -1)
	require.Len(t, found, 16)
	for _, m := range found {
		id := m[1] + m[2]
		assert.True(t, known[id], id)
	}
}

func sampleReport() Report {
	return Report{Skills: []string{"skills/a", "skills/b"}, Findings: []Finding{
		{Rule: "description-no-trigger", Category: Quality, Severity: Warning, Skill: "skills/a", File: "skills/a/SKILL.md", Line: 3, Message: "says what, not when"},
		{Rule: "missing-reference", Category: Quality, Severity: Error, Skill: "skills/a", File: "skills/a/SKILL.md", Line: 6, Message: "links a|b <x>.md, which does not exist"},
		{Rule: "pipe-to-shell", Category: Safety, Severity: Warning, Skill: "skills/b", File: "skills/b/run me.sh", Line: 4, Message: "pipes curl into sh"},
	}}
}

func TestWriteSARIF(t *testing.T) {
	var out strings.Builder
	require.NoError(t, sampleReport().WriteSARIF(&out))
	var log sarif.Log
	require.NoError(t, json.Unmarshal([]byte(out.String()), &log))
	require.Len(t, log.Runs, 1)
	run := log.Runs[0]
	assert.Equal(t, &sarif.AutomationDetails{ID: "zanadir/skills/"}, run.AutomationDetails, "a category apart from the scan's")
	assert.Len(t, run.Tool.Driver.Rules, len(ruleInfos))
	levels := map[string]string{}
	for _, rule := range run.Tool.Driver.Rules {
		levels[rule.ID] = rule.DefaultConfiguration.Level
	}
	assert.Equal(t, "error", levels["skills/missing-reference"])
	assert.Equal(t, "warning", levels["skills/pipe-to-shell"])

	require.Len(t, run.Results, 3)
	r := run.Results[1]
	assert.Equal(t, "skills/missing-reference", r.RuleID)
	assert.Equal(t, "error", r.Level)
	assert.Equal(t, "links a|b <x>.md, which does not exist", r.Message.Text)
	assert.Equal(t, sarif.At("skills/a/SKILL.md", 6), r.Locations)
	assert.Equal(t, "skills/b/run%20me.sh", run.Results[2].Locations[0].PhysicalLocation.ArtifactLocation.URI)
	assert.NotContains(t, out.String(), "partialFingerprints", "code scanning hashes the line itself")
}

func TestWriteMarkdown(t *testing.T) {
	var out strings.Builder
	require.NoError(t, sampleReport().WriteMarkdown(&out))
	assert.Equal(t, MarkdownMarker+"\n### zanadir skills\n\nChecked 2 skills: 1 error, 2 warnings.\n\n"+
		"| Severity | Problem | Where | Rule |\n| --- | --- | --- | --- |\n"+
		"| error | links a\\|b &lt;x&gt;.md, which does not exist | `skills/a/SKILL.md:6` | `missing-reference` |\n"+
		"| warning | says what, not when | `skills/a/SKILL.md:3` | `description-no-trigger` |\n"+
		"| warning | pipes curl into sh | `skills/b/run me.sh:4` | `pipe-to-shell` |\n", out.String())

	out.Reset()
	require.NoError(t, Report{Skills: []string{"a"}}.WriteMarkdown(&out))
	assert.Equal(t, MarkdownMarker+"\n### zanadir skills\n\nChecked 1 skill: no problems found.\n", out.String())

	out.Reset()
	require.NoError(t, Report{}.WriteMarkdown(&out))
	assert.Contains(t, out.String(), "No skills found (no SKILL.md files).")
}

func TestWriteMarkdownStopsAtItsLimit(t *testing.T) {
	report := Report{Skills: []string{"a"}}
	for i := range maxMarkdownFindings + 5 {
		report.Findings = append(report.Findings, Finding{Rule: "body-long", Severity: Warning, File: fmt.Sprintf("s%d/SKILL.md", i), Line: 1, Message: "long"})
	}
	report.Findings = append(report.Findings, Finding{Rule: "missing-reference", Severity: Error, File: "last/SKILL.md", Line: 2, Message: "gone"})
	var out strings.Builder
	require.NoError(t, report.WriteMarkdown(&out))
	assert.Equal(t, maxMarkdownFindings, strings.Count(out.String(), "\n| error")+strings.Count(out.String(), "\n| warning"))
	assert.Contains(t, out.String(), "| error | gone | `last/SKILL.md:2`", "errors come first, so the limit drops warnings")
	assert.Contains(t, out.String(), "_6 more not shown; run `zanadir skills` to see them all._")
}
