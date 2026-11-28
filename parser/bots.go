package parser

import (
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"

	"github.com/MustacheCase/zanadir/internal/repofs"
	"github.com/MustacheCase/zanadir/models"
	"github.com/MustacheCase/zanadir/utils"
)

// DependencyBotParser reads dependency-update bot configurations and fetched
// Dependabot security updates, which run outside CI jobs.
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

// Exists reports whether root configures a dependency bot or has fetched
// Dependabot security updates enabled.
func (b *DependencyBotParser) Exists(root string) bool {
	for _, bot := range botConfigs {
		if utils.FirstFile(root, bot.paths...) != "" {
			return true
		}
	}
	_, ok := readDependabotSecurityUpdates(root)
	return ok
}

func readDependabotSecurityUpdates(root string) ([]string, bool) {
	data, err := repofs.ReadFile(root, filepath.Join(root, filepath.FromSlash(models.DependabotSecurityUpdatesFile)))
	return strings.Fields(string(data)), err == nil
}

// Parse reports one artifact per bot that watches the project's dependencies.
// A config zanadir cannot read still counts: it cannot tell. Fetched security
// updates apply only when Dependabot has no config, and have no file location.
func (b *DependencyBotParser) Parse(root string) ([]*models.Artifact, error) {
	var artifacts []*models.Artifact

	for _, bot := range botConfigs {
		rel := utils.FirstFile(root, bot.paths...)
		location := ""
		if rel != "" {
			location = filepath.Join(root, rel)
			if onlyTooling(bot.tool, root, location) {
				continue
			}
		} else {
			if bot.tool != "dependabot" {
				continue
			}
			ecosystems, ok := readDependabotSecurityUpdates(root)
			if !ok || onlyToolingEcosystems(ecosystems) {
				continue
			}
		}
		artifacts = append(artifacts, &models.Artifact{
			Name:            bot.tool,
			Location:        location,
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
	return onlyToolingEcosystems(updated)
}

func onlyToolingEcosystems(updated []string) bool {
	return len(updated) > 0 && !slices.ContainsFunc(updated, func(e string) bool { return !slices.Contains(toolingEcosystems, e) })
}

// NewDependencyBotParser returns a Parser for Dependabot and Renovate configuration.
func NewDependencyBotParser() Parser {
	return &DependencyBotParser{}
}
