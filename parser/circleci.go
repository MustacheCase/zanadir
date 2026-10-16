package parser

import (
	"path/filepath"
	"strings"

	"github.com/MustacheCase/zanadir/models"
	"github.com/MustacheCase/zanadir/utils"
)

type CircleCIParser struct{}

type circleCIDef struct {
	Version   string                    `yaml:"version"`
	Orbs      map[string]string         `yaml:"orbs"`
	Jobs      map[string]circleCIJobDef `yaml:"jobs"`
	Workflows circleCIWorkflowsDef      `yaml:"workflows"`
}

type circleCIJobDef struct {
	Docker []map[string]string `yaml:"docker"`
	Steps  []interface{}       `yaml:"steps"`
}

type circleCIWorkflowDef struct {
	Jobs []string `yaml:"jobs"`
}

type circleCIWorkflowsDef struct {
	Version      int                 `yaml:"version"`
	TestAndBuild circleCIWorkflowDef `yaml:"test-and-build"`
}

func circleCIConfigs(root string) []string {
	return utils.YAMLFiles(filepath.Join(root, ".circleci"))
}

func (c *CircleCIParser) Exists(root string) bool {
	return len(circleCIConfigs(root)) > 0
}

func (c *CircleCIParser) Parse(root string) ([]*models.Artifact, error) {
	var artifacts []*models.Artifact
	for _, path := range circleCIConfigs(root) {
		artifact, err := c.parseCircleCIWorkflow(path)
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, artifact)
	}
	return artifacts, nil
}

func (c *CircleCIParser) parseCircleCIWorkflow(filePath string) (*models.Artifact, error) {
	var wf circleCIDef
	if err := utils.ReadYAML(filePath, &wf); err != nil {
		return nil, err
	}

	var jobs []*models.Job
	for orbName, orbDefinition := range wf.Orbs {
		pkgName, version := parseOrbDefinition(orbName, map[string]string{orbName: orbDefinition}) // Pass a map with the single orb
		if pkgName != "" {
			jobs = append(jobs, &models.Job{Name: orbName, Package: pkgName, Version: version})
		}
	}

	// Orbs are only half the picture; jobs also invoke tools from run steps.
	for jobName, job := range wf.Jobs {
		if commands := extractRunCommands(job.Steps); len(commands) > 0 {
			jobs = append(jobs, &models.Job{Name: jobName, Run: strings.Join(commands, "\n")})
		}
	}

	return &models.Artifact{
		Name:     "CircleCI Workflow Orbs",
		Jobs:     jobs,
		Location: filePath,
	}, nil
}

// extractRunCommands pulls shell command text out of a CircleCI steps list.
// A run step is either a plain command or a mapping with a "command" key.
func extractRunCommands(steps []interface{}) []string {
	var commands []string
	for _, step := range steps {
		stepMap, ok := step.(map[string]interface{})
		if !ok {
			continue
		}
		run, ok := stepMap["run"]
		if !ok {
			continue
		}
		switch v := run.(type) {
		case string:
			commands = append(commands, v)
		case map[string]interface{}:
			if command, ok := v["command"].(string); ok {
				commands = append(commands, command)
			}
		}
	}
	return commands
}

func parseOrbDefinition(orb string, orbs map[string]string) (string, string) {
	if definedOrb, exists := orbs[orb]; exists {
		fields := strings.Split(definedOrb, "@")
		if len(fields) == 2 {
			return fields[0], fields[1]
		}
		return definedOrb, ""
	}
	return "", ""
}
func NewCircleCIParser() Parser {
	return &CircleCIParser{}
}
