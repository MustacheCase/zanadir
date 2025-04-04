package parser

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/MustacheCase/zanadir/models"
	"gopkg.in/yaml.v3"
)

// GitLabParser struct for parsing GitLab CI/CD files
type GitLabParser struct{}

func (g *GitLabParser) Exists(location string) bool {
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

func (g *GitLabParser) Parse(location string) ([]*models.Artifact, error) {
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
			artifact, err := g.parseGitLabWorkflow(path)
			if err != nil {
				return nil, err
			}
			artifacts = append(artifacts, artifact)
		}
	}

	return artifacts, nil
}

func (g *GitLabParser) parseGitLabWorkflow(filePath string) (*models.Artifact, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var wf map[string]interface{}
	if err := yaml.Unmarshal(data, &wf); err != nil {
		return nil, err
	}

	var jobs []*models.Job
	if jobDefs, ok := wf["jobs"].(map[string]interface{}); ok {
		for jobName, jobDef := range jobDefs {
			if jobMap, ok := jobDef.(map[string]interface{}); ok {
				if script, ok := jobMap["script"].([]interface{}); ok {
					var scriptStrings []string
					for _, s := range script {
						if str, ok := s.(string); ok {
							scriptStrings = append(scriptStrings, str)
						}
					}
					jobs = append(jobs, &models.Job{Name: jobName, Package: "script", Version: strings.Join(scriptStrings, ", ")})
				}
			}
		}
	}

	return &models.Artifact{
		Name:     "GitLab CI/CD Workflow",
		Jobs:     jobs,
		Location: filePath,
	}, nil
}

func NewGitLabParser() Parser {
	return &GitLabParser{}
}
