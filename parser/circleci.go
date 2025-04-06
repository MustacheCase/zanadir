package parser

import (
	"os"
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

func (c *CircleCIParser) Exists(location string) bool {
	info, err := os.Stat(location)
	if err != nil || !info.IsDir() {
		return false
	}

	entries, err := os.ReadDir(location)
	if err != nil || len(entries) == 0 {
		return false
	}

	return true
}

func (c *CircleCIParser) Parse(location string) ([]*models.Artifact, error) {
	files, err := os.ReadDir(location)
	if err != nil {
		return nil, err
	}

	var artifacts []*models.Artifact
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		if filepath.Ext(file.Name()) == ".yml" || filepath.Ext(file.Name()) == ".yaml" {
			path := filepath.Join(location, file.Name())
			artifact, err := c.parseCircleCIWorkflow(path)
			if err != nil {
				return nil, err
			}
			print(artifact)
			artifacts = append(artifacts, artifact)
		}
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

	return &models.Artifact{
		Name:     "CircleCI Workflow Orbs",
		Jobs:     jobs,
		Location: filePath,
	}, nil
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
