package parser

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/MustacheCase/zanadir/models"
	"github.com/MustacheCase/zanadir/utils"
)

type GithubParser struct{}

type workflowDef struct {
	Name string
	Jobs map[string]workflowJobDef `yaml:"jobs"`
}

type workflowJobDef struct {
	Uses  string    `yaml:"uses"`
	Steps []stepDef `yaml:"steps"`
}

type stepDef struct {
	Name string `yaml:"name"`
	Uses string `yaml:"uses"`
	With struct {
		Path string `yaml:"path"`
		Key  string `yaml:"key"`
	} `yaml:"with"`
}

func (g *GithubParser) Exists(location string) bool {
	info, err := os.Stat(location)
	if err != nil || !info.IsDir() {
		return false
	}

	// Check if the directory is empty
	entries, err := os.ReadDir(location)
	if err != nil || len(entries) == 0 {
		return false
	}

	return true
}

func (g *GithubParser) Parse(location string) ([]*models.Artifact, error) {
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
			artifact, err := g.parseGithubWorkflow(path)
			if err != nil {
				return nil, err
			}
			artifacts = append(artifacts, artifact)
		}
	}

	return artifacts, nil
}

func (g *GithubParser) parseGithubWorkflow(filePath string) (*models.Artifact, error) {
	var wf workflowDef
	if err := utils.ReadYAML(filePath, &wf); err != nil {
		return nil, err
	}

	var jobs []*models.Job
	for jobName, job := range wf.Jobs {
		for _, step := range job.Steps {
			if step.Uses == "" {
				continue
			}
			pkgName, version := parseStepUsageStatement(step.Uses)
			if pkgName != "" {
				jobs = append(jobs, &models.Job{Name: jobName, Package: pkgName, Version: version})
			}
		}
	}

	return &models.Artifact{
		Name:     wf.Name,
		Jobs:     jobs,
		Location: filePath,
	}, nil
}

func parseStepUsageStatement(use string) (string, string) {
	// from octo-org/another-repo/.github/workflows/workflow.yml@v1 get octo-org/another-repo/.github/workflows/workflow.yml and v1
	// from ./.github/workflows/workflow-2.yml interpret as only the name

	// from actions/cache@v3 get actions/cache and v3

	fields := strings.Split(use, "@")
	switch len(fields) {
	case 1:
		return use, ""
	case 2:
		return fields[0], fields[1]
	}
	return "", ""
}

func NewGithubParser() Parser {
	return &GithubParser{}
}
