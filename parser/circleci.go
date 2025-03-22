package parser

import (
	"github.com/MustacheCase/zanadir/models"
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
	"strings"
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
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var wf circleCIDef
	if err := yaml.Unmarshal(data, &wf); err != nil {
		return nil, err
	}

	var jobs []*models.Job
	for jobName, job := range wf.Jobs {
		for _, step := range job.Steps {
			switch v := step.(type) {
			case string:
				pkgName, version := parseOrbDefinition(v, wf.Orbs)
				jobs = append(jobs, &models.Job{Name: jobName, Package: pkgName, Version: version})
			case map[string]interface{}:
				if orb, ok := v["orb"].(string); ok {
					pkgName, version := parseOrbDefinition(orb, wf.Orbs)
					jobs = append(jobs, &models.Job{Name: jobName, Package: pkgName, Version: version})
				} else if uses, ok := v["uses"].(string); ok {
					pkgName, version := parseOrbDefinition(uses, wf.Orbs)
					jobs = append(jobs, &models.Job{Name: jobName, Package: pkgName, Version: version})
				} else { // Check if the map key is an orb
					for key := range v {
						pkgName, version := parseOrbDefinition(key, wf.Orbs)
						if pkgName != "" {
							jobs = append(jobs, &models.Job{Name: jobName, Package: pkgName, Version: version})
							break // Only process the orb key
						}
					}
				}
			}
		}
	}

	return &models.Artifact{
		Name:     "CircleCI Workflow",
		Jobs:     jobs,
		Location: filePath,
	}, nil
}
func parseOrbDefinition(orb string, orbs map[string]string) (string, string) {
	fields := strings.Split(orb, "/")
	if len(fields) >= 2 {
		orbName := fields[0]
		if definedOrb, exists := orbs[orbName]; exists {
			fields := strings.Split(definedOrb, "@")
			if len(fields) == 2 {
				return fields[0], fields[1]
			}
			return definedOrb, ""
		}
	}
	return "", "" // Return empty strings if not found
}
func NewCircleCIParser() Parser {
	return &CircleCIParser{}
}
