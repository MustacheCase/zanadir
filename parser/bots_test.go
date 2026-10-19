package parser_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/MustacheCase/zanadir/internal/testrepo"
	"github.com/MustacheCase/zanadir/parser"
	"github.com/stretchr/testify/assert"
)

func TestDependencyBotParserFindsEveryKnownPath(t *testing.T) {
	tests := []struct {
		name         string
		file         string
		expectedTool string
	}{
		{"dependabot yml", ".github/dependabot.yml", "dependabot"},
		{"dependabot yaml", ".github/dependabot.yaml", "dependabot"},
		{"renovate at the root", "renovate.json", "renovate"},
		{"renovate under .github", ".github/renovate.json", "renovate"},
		{"renovate under .gitlab", ".gitlab/renovate.json", "renovate"},
		{"renovaterc", ".renovaterc", "renovate"},
		{"renovaterc json", ".renovaterc.json", "renovate"},
		{"renovate json5", "renovate.json5", "renovate"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := testrepo.Write(t, map[string]string{tt.file: "{}"})
			p := parser.NewDependencyBotParser()

			assert.True(t, p.Exists(dir))

			artifacts, err := p.Parse(dir)
			assert.NoError(t, err)
			assert.Len(t, artifacts, 1)
			assert.Equal(t, tt.expectedTool, artifacts[0].Name)
			assert.Equal(t, filepath.Join(dir, tt.file), artifacts[0].Location)
			// The control runs on the forge's schedule, never in the pipeline.
			assert.True(t, artifacts[0].OutsidePipeline)
			assert.Len(t, artifacts[0].Jobs, 1)
			assert.Equal(t, tt.expectedTool, artifacts[0].Jobs[0].Package)
		})
	}
}

func TestDependencyBotParserWithBothBots(t *testing.T) {
	dir := testrepo.Write(t, map[string]string{
		".github/dependabot.yml": "version: 2",
		"renovate.json":          "{}",
	})

	artifacts, err := parser.NewDependencyBotParser().Parse(dir)
	assert.NoError(t, err)
	assert.Len(t, artifacts, 2)
	assert.Equal(t, "dependabot", artifacts[0].Name)
	assert.Equal(t, "renovate", artifacts[1].Name)
}

func TestDependencyBotParserWithNoConfig(t *testing.T) {
	dir := testrepo.Write(t, map[string]string{".github/workflows/ci.yml": "name: ci"})
	p := parser.NewDependencyBotParser()

	assert.False(t, p.Exists(dir))

	artifacts, err := p.Parse(dir)
	assert.NoError(t, err)
	assert.Empty(t, artifacts)
}

// A directory named like a config is not a config.
func TestDependencyBotParserIgnoresDirectories(t *testing.T) {
	dir := t.TempDir()
	assert.NoError(t, os.MkdirAll(filepath.Join(dir, "renovate.json"), 0o755))

	assert.False(t, parser.NewDependencyBotParser().Exists(dir))
}

// Renovate reads the first config it finds, so only one is reported even when
// a repository has left several lying about.
func TestDependencyBotParserReportsOneArtifactPerBot(t *testing.T) {
	dir := testrepo.Write(t, map[string]string{
		"renovate.json":         "{}",
		".github/renovate.json": "{}",
		".renovaterc":           "{}",
	})

	artifacts, err := parser.NewDependencyBotParser().Parse(dir)
	assert.NoError(t, err)
	assert.Len(t, artifacts, 1)
	assert.Equal(t, filepath.Join(dir, "renovate.json"), artifacts[0].Location)
}

// A bot that only bumps actions or the dev container watches none of the
// code's dependencies, so it is no dependency scanning.
func TestDependencyBotsThatOnlyUpdateTooling(t *testing.T) {
	bots := func(files map[string]string) []string {
		artifacts, err := parser.NewDependencyBotParser().Parse(testrepo.Write(t, files))
		assert.NoError(t, err)
		var names []string
		for _, a := range artifacts {
			names = append(names, a.Name)
		}
		return names
	}
	actions := "version: 2\nupdates:\n  - package-ecosystem: github-actions\n    directory: /\n  - package-ecosystem: devcontainers\n    directory: /\n"
	gomod := actions + "  - package-ecosystem: gomod\n    directory: /\n"

	assert.Empty(t, bots(map[string]string{".github/dependabot.yml": actions}))
	assert.Equal(t, []string{"dependabot"}, bots(map[string]string{".github/dependabot.yml": gomod}))
	assert.Equal(t, []string{"dependabot"}, bots(map[string]string{".github/dependabot.yml": "version: 2\n"}), "no updates listed: cannot tell")
	assert.Equal(t, []string{"dependabot"}, bots(map[string]string{".github/dependabot.yml": "updates: [oops"}), "unreadable: cannot tell")

	assert.Empty(t, bots(map[string]string{"renovate.json": `{"enabledManagers": ["github-actions"]}`}))
	assert.Equal(t, []string{"renovate"}, bots(map[string]string{"renovate.json": `{"enabledManagers": ["github-actions", "npm"]}`}))
	assert.Equal(t, []string{"renovate"}, bots(map[string]string{"renovate.json": `{"extends": ["config:recommended"]}`}))
	assert.Equal(t, []string{"renovate"}, bots(map[string]string{"renovate.json5": "{enabledManagers: ['github-actions']}"}), "JSON5 is not read")
}
