package skills

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunAndTable(t *testing.T) {
	root := t.TempDir()
	write(t, root, "skills/a/SKILL.md", skill("a", "Does a thing well enough to describe at length.", "")+"[x](gone.md)\n")

	report, err := Run(root)
	require.NoError(t, err)
	assert.Equal(t, 1, report.Errors())

	var out strings.Builder
	require.NoError(t, report.WriteTable(&out))
	assert.Equal(t, "Found 1 skill: skills/a\n\nQuality\n"+
		"  ! skills/a/SKILL.md:3  description says what the skill does but not when to use it; add a \"Use when ...\" sentence  [description-no-trigger]\n"+
		"  \u2717 skills/a/SKILL.md:6  links gone.md, which does not exist  [missing-reference]\n"+
		"\n1 error, 1 warning\n", out.String())
}

func TestTableWithoutProblems(t *testing.T) {
	var out strings.Builder
	require.NoError(t, Report{Skills: []string{"a", "b"}}.WriteTable(&out))
	assert.Equal(t, "Found 2 skills: a, b\n\nNo problems found.\n", out.String())
}

func TestTableWithoutSkills(t *testing.T) {
	var out strings.Builder
	require.NoError(t, Report{}.WriteTable(&out))
	assert.Equal(t, "No skills found (no SKILL.md files).\n", out.String())
}

func TestTableGroupsSafetyAfterQuality(t *testing.T) {
	var out strings.Builder
	report := Report{Skills: []string{"a"}, Findings: []Finding{
		{Rule: "pipe-to-shell", Category: Safety, Severity: Warning, File: "a/s.sh", Line: 2, Message: "m"},
		{Rule: "frontmatter-invalid", Category: Quality, Severity: Error, File: "a/SKILL.md", Line: 1, Message: "q"},
		{Rule: "pipe-to-shell", Category: Safety, Severity: Warning, File: "a/t.sh", Line: 1, Message: "n"},
	}}
	require.NoError(t, report.WriteTable(&out))
	assert.Contains(t, out.String(), "\nQuality\n  \u2717 a/SKILL.md:1  q  [frontmatter-invalid]\n\nSafety\n  ! a/s.sh:2  m  [pipe-to-shell]\n  ! a/t.sh:1  n  [pipe-to-shell]\n\n1 error, 2 warnings\n")
}

func TestJSONIsAnEmptyListWhenClean(t *testing.T) {
	report, err := Run(t.TempDir())
	require.NoError(t, err)

	var out strings.Builder
	require.NoError(t, report.WriteJSON(&out))
	var decoded map[string]any
	require.NoError(t, json.Unmarshal([]byte(out.String()), &decoded))
	assert.Equal(t, []any{}, decoded["skills"])
	assert.Equal(t, []any{}, decoded["findings"])
}

func TestRunFailsOnAMissingDirectory(t *testing.T) {
	_, err := Run(filepath.Join(t.TempDir(), "absent"))
	assert.Error(t, err)
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("closed") }

func TestWriteErrors(t *testing.T) {
	assert.Error(t, Report{}.WriteTable(failingWriter{}))
	assert.Error(t, Report{}.WriteJSON(failingWriter{}))
}
