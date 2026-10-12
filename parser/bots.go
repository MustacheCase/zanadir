package parser

import (
	"os"
	"path/filepath"

	"github.com/MustacheCase/zanadir/models"
)

// DependencyBotParser reads the dependency-update bots, which declare
// themselves in a configuration file rather than in a CI job. Nothing else
// read them, so a repository whose dependency updates come from Dependabot was
// told it had no software composition analysis at all.
type DependencyBotParser struct{}

// botConfig is one bot and the paths it may be configured at, relative to the
// repository root. Renovate accepts many names and reads the first it finds.
type botConfig struct {
	tool  string
	paths []string
}

var botConfigs = []botConfig{
	{
		tool:  "dependabot",
		paths: []string{".github/dependabot.yml", ".github/dependabot.yaml"},
	},
	{
		tool: "renovate",
		paths: []string{
			"renovate.json",
			"renovate.json5",
			".github/renovate.json",
			".github/renovate.json5",
			".gitlab/renovate.json",
			".renovaterc",
			".renovaterc.json",
			".renovaterc.json5",
		},
	},
}

// found returns the first configured path for a bot, relative to the root.
func (b *DependencyBotParser) found(root string, bot botConfig) string {
	for _, rel := range bot.paths {
		info, err := os.Stat(filepath.Join(root, rel))
		if err == nil && !info.IsDir() {
			return rel
		}
	}
	return ""
}

func (b *DependencyBotParser) Exists(root string) bool {
	for _, bot := range botConfigs {
		if b.found(root, bot) != "" {
			return true
		}
	}
	return false
}

// Parse reports one artifact per configured bot. The contents are not read:
// the file existing is the control, and its ecosystems say which manifests are
// watched, not whether anything is watched at all.
func (b *DependencyBotParser) Parse(root string) ([]*models.Artifact, error) {
	var artifacts []*models.Artifact

	for _, bot := range botConfigs {
		rel := b.found(root, bot)
		if rel == "" {
			continue
		}
		artifacts = append(artifacts, &models.Artifact{
			Name:            bot.tool,
			Location:        filepath.Join(root, rel),
			OutsidePipeline: true,
			// The tool name is the whole of the match, so the rule can anchor
			// on it: "dependabot/fetch-metadata" is a real action that has
			// nothing to do with dependency updates.
			Jobs: []*models.Job{{Name: bot.tool, Package: bot.tool}},
		})
	}

	return artifacts, nil
}

func NewDependencyBotParser() Parser {
	return &DependencyBotParser{}
}
