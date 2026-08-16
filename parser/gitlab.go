package parser

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/MustacheCase/zanadir/models"
	"github.com/MustacheCase/zanadir/utils"
)

type GitlabParser struct{}

type gitlabCIConfig struct {
	Stages []string                `yaml:"stages"`
	Jobs   map[string]gitlabJobDef `yaml:",inline"`
}

type gitlabJobDef struct {
	Stage        string   `yaml:"stage"`
	BeforeScript []string `yaml:"before_script"`
	Script       []string `yaml:"script"`
	AfterScript  []string `yaml:"after_script"`
	Image        string   `yaml:"image"`
	Services     []string `yaml:"services"`
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
		// Script blocks carry the same signal `uses:` does for GitHub Actions.
		script := make([]string, 0, len(job.BeforeScript)+len(job.Script)+len(job.AfterScript))
		script = append(script, job.BeforeScript...)
		script = append(script, job.Script...)
		script = append(script, job.AfterScript...)

		jobs = append(jobs, &models.Job{
			Name:    jobName,
			Package: job.Image,
			Version: "",
			Run:     strings.Join(script, "\n"),
		})
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
