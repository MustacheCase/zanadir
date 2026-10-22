package parser

import (
	"encoding/json"
	"path/filepath"
	"slices"

	"github.com/MustacheCase/zanadir/internal/repofs"
	"github.com/MustacheCase/zanadir/models"
	"github.com/MustacheCase/zanadir/utils"
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

// Exists reports whether root configures Dependabot or Renovate.
func (b *DependencyBotParser) Exists(root string) bool {
	for _, bot := range botConfigs {
		if utils.FirstFile(root, bot.paths...) != "" {
			return true
		}
	}
	return false
}

// Parse reports one artifact per configured bot that watches the project's
// dependencies. A file zanadir cannot read still counts: it cannot tell.
func (b *DependencyBotParser) Parse(root string) ([]*models.Artifact, error) {
	var artifacts []*models.Artifact

	for _, bot := range botConfigs {
		rel := utils.FirstFile(root, bot.paths...)
		if rel == "" || onlyTooling(bot.tool, root, filepath.Join(root, rel)) {
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

// toolingEcosystems keep the CI and the dev container current rather than the
// code's dependencies, which is what the bots are counted for.
var toolingEcosystems = []string{"github-actions", "devcontainers"}

// onlyTooling reports whether a bot's configuration updates nothing but
// tooling: Dependabot's package-ecosystem entries, or Renovate's
// enabledManagers in a plain-JSON file.
func onlyTooling(tool, root, path string) bool {
	var updated []string
	switch tool {
	case "dependabot":
		var config struct {
			Updates []struct {
				Ecosystem string `yaml:"package-ecosystem"`
			} `yaml:"updates"`
		}
		if readYAML(root, path, &config) != nil {
			return false
		}
		for _, update := range config.Updates {
			updated = append(updated, update.Ecosystem)
		}
	default:
		var config struct {
			EnabledManagers []string `json:"enabledManagers"`
		}
		data, err := repofs.ReadFile(root, path)
		if err != nil || json.Unmarshal(data, &config) != nil {
			return false
		}
		updated = config.EnabledManagers
	}
	return len(updated) > 0 && !slices.ContainsFunc(updated, func(e string) bool { return !slices.Contains(toolingEcosystems, e) })
}

// NewDependencyBotParser returns a Parser for Dependabot and Renovate configuration.
func NewDependencyBotParser() Parser {
	return &DependencyBotParser{}
}
