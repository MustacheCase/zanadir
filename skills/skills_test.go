package skills

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func write(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

const good = "---\nname: deploy\ndescription: Deploys the service to staging. Use when the user asks to ship or deploy a branch.\n---\n# Deploy\n"

func TestDiscoverFindsSkillsAndSkipsDependenciesAndTests(t *testing.T) {
	root := t.TempDir()
	write(t, root, ".claude/skills/deploy/SKILL.md", good)
	write(t, root, "plugins/acme/skills/review/SKILL.md", good)
	write(t, root, "node_modules/pkg/skills/x/SKILL.md", good)
	write(t, root, "testdata/skills/y/SKILL.md", good)
	write(t, root, ".claude/skills/notes/README.md", good)

	found, err := Discover(root)

	require.NoError(t, err)
	assert.Equal(t, []string{".claude/skills/deploy", "plugins/acme/skills/review"}, Names(found))
}

func TestDiscoverDoesNotFollowASymlinkOutOfTheRepository(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	write(t, outside, "SKILL.md", good)
	require.NoError(t, os.MkdirAll(filepath.Join(root, "skills/evil"), 0o755))
	require.NoError(t, os.Symlink(filepath.Join(outside, "SKILL.md"), filepath.Join(root, "skills/evil/SKILL.md")))

	found, err := Discover(root)

	require.NoError(t, err)
	assert.Empty(t, found)
}

func TestDiscoverReportsAMissingDirectory(t *testing.T) {
	_, err := Discover(filepath.Join(t.TempDir(), "absent"))
	assert.Error(t, err)
}

func TestParseReadsFrontmatter(t *testing.T) {
	s := Parse("skills/a/SKILL.md", []byte("\uFEFF---\r\nname: a\r\ndescription: >\r\n  Folded text.\r\nallowed-tools:\r\n  - Read\r\n  - Bash\r\n---\r\nBody\r\n"))

	assert.True(t, s.HasFrontmatter)
	assert.NoError(t, s.FrontmatterErr)
	assert.Equal(t, "a", s.Name)
	assert.Equal(t, "Folded text.", s.Description)
	assert.Equal(t, "Read, Bash", s.AllowedTools)
	assert.Equal(t, "Body\n", s.Body)
	assert.Equal(t, "skills/a", s.Dir)
	assert.Equal(t, 2, s.line("name"))
	assert.Equal(t, 1, s.line("license"))
}

func TestParseFrontmatterClosedAtTheEndOfTheFile(t *testing.T) {
	s := Parse("SKILL.md", []byte("---\nname: a\n---"))
	assert.True(t, s.HasFrontmatter)
	assert.Equal(t, "a", s.Name)
	assert.Equal(t, ".", s.Dir)
}

func TestParseWithoutFrontmatter(t *testing.T) {
	assert.False(t, Parse("a/SKILL.md", []byte("# Title\n")).HasFrontmatter)
	assert.False(t, Parse("a/SKILL.md", []byte("---\nname: a\nnever closed\n")).HasFrontmatter)
}

func TestParseInvalidFrontmatter(t *testing.T) {
	assert.Error(t, Parse("a/SKILL.md", []byte("---\nname: [a\n---\n")).FrontmatterErr)
	assert.EqualError(t, Parse("a/SKILL.md", []byte("---\nname: a\ndescription: [x, y]\n---\n")).FrontmatterErr, "description must be a string")
}

func TestToolsText(t *testing.T) {
	assert.Equal(t, "Read Bash", toolsText("Read Bash"))
	assert.Equal(t, "", toolsText(7))
}

func TestScope(t *testing.T) {
	assert.Equal(t, ".claude/skills", (&Skill{Dir: ".claude/skills/a"}).scope())
	assert.Equal(t, "plugins/x/skills", (&Skill{Dir: "plugins/x/skills/a/skills-b"}).scope())
	assert.Equal(t, "", (&Skill{Dir: "tools/a"}).scope())
}

func TestPresent(t *testing.T) {
	root := t.TempDir()
	write(t, root, "node_modules/pkg/skills/a/SKILL.md", good)
	write(t, root, "tests/skills/b/SKILL.md", good)
	assert.False(t, Present(root))

	write(t, root, "plugins/acme/skills/c/SKILL.md", "")
	assert.True(t, Present(root))

	assert.False(t, Present(filepath.Join(root, "absent")))
}
