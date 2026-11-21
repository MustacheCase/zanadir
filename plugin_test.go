package main

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	fullPlugin      = "plugins/zanadir"
	directoryPlugin = "directory/zanadir"
	skill           = "skills/improve-ci/SKILL.md"
)

var frontmatterField = regexp.MustCompile(`(?m)^(name|description|allowed-tools): (.*)$`)

func readSkill(t *testing.T, plugin string) (map[string]string, string) {
	data, err := os.ReadFile(filepath.Join(plugin, skill))
	require.NoError(t, err)
	parts := strings.SplitN(strings.ReplaceAll(string(data), "\r\n", "\n"), "---\n", 3)
	require.Len(t, parts, 3, plugin)
	fields := map[string]string{}
	for _, m := range frontmatterField.FindAllStringSubmatch(parts[1], -1) {
		fields[m[1]] = m[2]
	}
	return fields, parts[2]
}

// The directory edition is the full skill without the MCP server: the same
// trigger, and the same steps from evidence gathering on.
func TestDirectorySkillFollowsTheFullSkill(t *testing.T) {
	full, fullBody := readSkill(t, fullPlugin)
	slim, slimBody := readSkill(t, directoryPlugin)
	assert.Equal(t, full["name"], slim["name"])
	assert.Equal(t, full["description"], slim["description"])

	from := "## 2. Gather evidence\n"
	require.Contains(t, fullBody, from)
	require.Contains(t, slimBody, from)
	assert.Equal(t, fullBody[strings.Index(fullBody, from):], slimBody[strings.Index(slimBody, from):])

	var manifests [2]struct{ Name string }
	for i, plugin := range []string{fullPlugin, directoryPlugin} {
		data, err := os.ReadFile(filepath.Join(plugin, ".claude-plugin/plugin.json"))
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(data, &manifests[i]))
	}
	assert.Equal(t, manifests[0].Name, manifests[1].Name)
}

// The directory takes from an unverified organisation only Markdown, text,
// JSON, YAML and images, no file starting with #!, and allowed-tools of
// Read, Grep, Glob and LS.
func TestDirectoryEditionMeetsTheCommunityRules(t *testing.T) {
	slim, _ := readSkill(t, directoryPlugin)
	for _, tool := range strings.Split(slim["allowed-tools"], ",") {
		assert.Contains(t, []string{"Read", "Grep", "Glob", "LS"}, strings.TrimSpace(tool))
	}

	allowed := map[string]bool{".md": true, ".txt": true, ".json": true, ".yaml": true, ".yml": true, ".png": true}
	require.NoError(t, filepath.WalkDir(directoryPlugin, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		name := d.Name()
		assert.True(t, allowed[filepath.Ext(name)] || name == "LICENSE", path)
		assert.NotContains(t, []string{".mcp.json", "hooks.json", ".lsp.json", "monitors.json"}, name, path)
		data, err := os.ReadFile(path)
		if err == nil {
			assert.False(t, strings.HasPrefix(string(data), "#!"), path)
		}
		return err
	}))
}
