package parser

import (
	"os"
	"path/filepath"

	"github.com/MustacheCase/zanadir/models"
)

// DependencyBotParser reads the dependency-update bots, which declare
// themselves in a configuration file rather than a CI job.
type DependencyBotParser struct{}

// Renovate accepts many names and reads the first it finds.
var botConfigs = []struct {
	tool  string
	paths []string
}{
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

func found(root string, paths []string) string {
	for _, rel := range paths {
		info, err := os.Stat(filepath.Join(root, rel))
		if err == nil && !info.IsDir() {
			return rel
		}
	}
	return ""
}

func (b *DependencyBotParser) Exists(root string) bool {
	for _, bot := range botConfigs {
		if found(root, bot.paths) != "" {
			return true
		}
	}
	return false
}

// Parse reports one artifact per configured bot. The contents are not read:
// the file existing is the control.
func (b *DependencyBotParser) Parse(root string) ([]*models.Artifact, error) {
	var artifacts []*models.Artifact

	for _, bot := range botConfigs {
		rel := found(root, bot.paths)
		if rel == "" {
			continue
		}
		artifacts = append(artifacts, &models.Artifact{
			Name:            bot.tool,
			Location:        filepath.Join(root, rel),
			OutsidePipeline: true,
			// The tool name is the whole package so the rule can anchor on it.
			Jobs: []*models.Job{{Name: bot.tool, Package: bot.tool}},
		})
	}

	return artifacts, nil
}

func NewDependencyBotParser() Parser {
	return &DependencyBotParser{}
}
