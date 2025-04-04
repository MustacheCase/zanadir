package parser

import (
	"os"
	"path/filepath"

	"github.com/MustacheCase/zanadir/models"
	"github.com/MustacheCase/zanadir/utils"
)

type GitlabParser struct{}

type gitlabCIConfig struct {
	Stages []string                `yaml:"stages"`
	Jobs   map[string]gitlabJobDef `yaml:",inline"`
}

type gitlabJobDef struct {
	Stage    string   `yaml:"stage"`
	Script   []string `yaml:"script"`
	Image    string   `yaml:"image"`
	Services []string `yaml:"services"`
}

func (g *GitlabParser) Exists(location string) bool {
	path := filepath.Join(location, ".gitlab-ci.yml")
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func (g *GitlabParser) Parse(location string) ([]*models.Artifact, error) {
	path := filepath.Join(location, ".gitlab-ci.yml")
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}

	var config gitlabCIConfig
	if err := utils.ReadYAML(path, &config); err != nil {
		return nil, err
	}

	var jobs []*models.Job
	for jobName, job := range config.Jobs {
		jobs = append(jobs, &models.Job{Name: jobName, Package: job.Image, Version: ""})
	}

	artifact := &models.Artifact{
		Name:     "GitLab CI/CD",
		Jobs:     jobs,
		Location: path,
	}

	return []*models.Artifact{artifact}, nil
}

func NewGitlabParser() Parser {
	return &GitlabParser{}
}
