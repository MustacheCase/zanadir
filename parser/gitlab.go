package parser

import (
	"bytes"
	"errors"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/MustacheCase/zanadir/models"
	"github.com/MustacheCase/zanadir/utils"
	"gopkg.in/yaml.v3"
)

type GitlabParser struct{}

const gitlabFile = ".gitlab-ci.yml"

// Included files nest; this bounds a chain that includes itself.
const gitlabIncludeDepth = 8

// gitlabKeywords configure the pipeline rather than name a job. The last five
// are deprecated globals that still appear in real files.
var gitlabKeywords = map[string]bool{
	"default": true, "include": true, "stages": true, "variables": true, "workflow": true, "spec": true,
	"image": true, "services": true, "cache": true, "before_script": true, "after_script": true,
}

// gitlabJob holds only what grading reads. Every field is a node because each
// takes more than one shape - a script is a line, a list, or lists nested by
// !reference - and one unreadable value must not abort the scan.
type gitlabJob struct {
	BeforeScript yaml.Node `yaml:"before_script"`
	Script       yaml.Node `yaml:"script"`
	AfterScript  yaml.Node `yaml:"after_script"`
	Image        yaml.Node `yaml:"image"`
	AllowFailure yaml.Node `yaml:"allow_failure"`
	When         yaml.Node `yaml:"when"`
}

type gitlabInclude struct {
	Local     string `yaml:"local"`
	Template  string `yaml:"template"`
	Component string `yaml:"component"`
}

type gitlabReader struct {
	root  string
	depth int
}

func (g *GitlabParser) Exists(root string) bool {
	return utils.IsFile(filepath.Join(root, gitlabFile))
}

func (g *GitlabParser) Parse(root string) ([]*models.Artifact, error) {
	path := filepath.Join(root, gitlabFile)
	config, err := readGitlabConfig(path)
	if err != nil {
		return nil, err
	}
	r := &gitlabReader{root: root}
	return []*models.Artifact{{
		Name:     "GitLab CI/CD",
		Jobs:     r.jobs(config),
		Location: path,
	}}, nil
}

// readGitlabConfig returns a file's top-level keys. A file may open with a
// spec: header as its own YAML document, so every document is read.
func readGitlabConfig(path string) (map[string]yaml.Node, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	config := map[string]yaml.Node{}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	for {
		var doc map[string]yaml.Node
		if err := decoder.Decode(&doc); errors.Is(err, io.EOF) {
			return config, nil
		} else if err != nil {
			return nil, err
		}
		maps.Copy(config, doc)
	}
}

func (r *gitlabReader) jobs(config map[string]yaml.Node) []*models.Job {
	include := config["include"]
	jobs := r.includes(&include)
	for _, name := range slices.Sorted(maps.Keys(config)) {
		if gitlabKeywords[name] {
			continue
		}
		node := config[name]
		if job := gitlabJobFrom(name, &node); job != nil {
			jobs = append(jobs, job)
		}
	}
	return jobs
}

func gitlabJobFrom(name string, node *yaml.Node) *models.Job {
	var job gitlabJob
	node = resolve(node)
	if node.Kind != yaml.MappingNode || node.Decode(&job) != nil {
		return nil
	}

	// Script blocks carry the same signal `uses:` does for GitHub Actions.
	var script []string
	for _, block := range []*yaml.Node{&job.BeforeScript, &job.Script, &job.AfterScript} {
		script = append(script, scriptLines(block)...)
	}
	return &models.Job{
		Name:            name,
		Package:         imageName(&job.Image),
		Run:             strings.Join(script, "\n"),
		ContinueOnError: scalarIs(&job.AllowFailure, "true"),
		Manual:          scalarIs(&job.When, "manual"),
	}
}

// includes reads include:, which takes a path, a list, or mappings. A local
// file is read as more of this configuration. A GitLab template or CI/CD
// component cannot be read, so it is recorded by name for the rules to match;
// a project or remote include cannot be read at all and is skipped.
func (r *gitlabReader) includes(node *yaml.Node) []*models.Job {
	node = resolve(node)
	entries := []*yaml.Node{node}
	if node.Kind == yaml.SequenceNode {
		entries = node.Content
	}

	var jobs []*models.Job
	for _, entry := range entries {
		entry = resolve(entry)
		var include gitlabInclude
		switch {
		case entry.Kind == yaml.ScalarNode:
			include.Local = entry.Value
		case entry.Kind != yaml.MappingNode || entry.Decode(&include) != nil:
			continue
		}

		switch {
		case include.Local != "":
			jobs = append(jobs, r.local(include.Local)...)
		case include.Template != "":
			jobs = append(jobs, &models.Job{Name: "include", Package: include.Template})
		case include.Component != "":
			jobs = append(jobs, &models.Job{Name: "include", Package: include.Component})
		}
	}
	return jobs
}

// local reads files included from this repository. GitLab resolves them from
// the repository root and allows wildcards; a remote URL or a path built from a
// variable cannot be read here.
func (r *gitlabReader) local(pattern string) []*models.Job {
	if strings.Contains(pattern, "://") || strings.Contains(pattern, "$") || r.depth >= gitlabIncludeDepth {
		return nil
	}
	matches, _ := filepath.Glob(filepath.Join(r.root, pattern))

	r.depth++
	defer func() { r.depth-- }()
	var jobs []*models.Job
	for _, path := range matches {
		if rel, err := filepath.Rel(r.root, path); err != nil || strings.HasPrefix(rel, "..") {
			continue
		}
		if config, err := readGitlabConfig(path); err == nil {
			jobs = append(jobs, r.jobs(config)...)
		}
	}
	return jobs
}

func NewGitlabParser() Parser {
	return &GitlabParser{}
}
