package parser

import (
	"path/filepath"
	"strings"

	"github.com/MustacheCase/zanadir/models"
	"github.com/MustacheCase/zanadir/utils"
	"gopkg.in/yaml.v3"
)

type CircleCIParser struct{}

// circleCIDef holds only what is read. Fields like docker: and workflows: take
// shapes a fixed type cannot, and decoding them anyway aborted whole scans.
type circleCIDef struct {
	Orbs map[string]yaml.Node      `yaml:"orbs"`
	Jobs map[string]circleCIJobDef `yaml:"jobs"`
}

type circleCIJobDef struct {
	Steps []interface{} `yaml:"steps"`
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
	if err := readYAML(filePath, &wf); err != nil {
		return unreadable(filePath, err), nil
	}

	var jobs []*models.Job
	for name, orb := range wf.Orbs {
		// An orb defined inline is a mapping, not a reference to a published one.
		if ref := resolve(&orb); ref.Kind == yaml.ScalarNode && ref.Value != "" {
			pkg, version, _ := strings.Cut(ref.Value, "@")
			jobs = append(jobs, &models.Job{Name: name, Package: pkg, Version: version})
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

func NewCircleCIParser() Parser {
	return &CircleCIParser{}
}
