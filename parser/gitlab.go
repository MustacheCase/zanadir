package parser

import (
	"bytes"
	"errors"
	"io"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"github.com/MustacheCase/zanadir/internal/repofs"
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
	Extends      yaml.Node `yaml:"extends"`
}

const gitlabExtendsDepth = 11

type gitlabInclude struct {
	Local     string `yaml:"local"`
	Template  string `yaml:"template"`
	Component string `yaml:"component"`
}

type gitlabReader struct {
	root        string
	depth       int
	definitions map[string]*yaml.Node
	included    []*models.Job
}

func (g *GitlabParser) Exists(root string) bool {
	return utils.IsFile(filepath.Join(root, gitlabFile))
}

func (g *GitlabParser) Parse(root string) ([]*models.Artifact, error) {
	path := filepath.Join(root, gitlabFile)
	config, err := readGitlabConfig(root, path)
	if err != nil {
		return skippedFile(path, err)
	}
	r := &gitlabReader{root: root, definitions: map[string]*yaml.Node{}}
	r.collect(config)
	return []*models.Artifact{{
		Name:     "GitLab CI/CD",
		Jobs:     r.jobs(),
		Location: path,
	}}, nil
}

// readGitlabConfig returns a file's top-level keys. A file may open with a
// spec: header as its own YAML document, so every document is read.
func readGitlabConfig(root, path string) (map[string]yaml.Node, error) {
	data, err := repofs.ReadFile(root, path)
	if err != nil {
		return nil, err
	}
	config := map[string]yaml.Node{}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	for {
		var doc map[string]yaml.Node
		if err := decodeNext(decoder, &doc); errors.Is(err, io.EOF) {
			return config, nil
		} else if err != nil {
			return nil, err
		}
		maps.Copy(config, doc)
	}
}

// collect records a file's jobs after those of the files it includes, so
// its own definitions win, as they do in GitLab.
func (r *gitlabReader) collect(config map[string]yaml.Node) {
	include := config["include"]
	r.includes(&include)
	for name, node := range config {
		if !gitlabKeywords[name] {
			r.definitions[name] = &node
		}
	}
}

// jobs are the jobs that run: a hidden job, named with a leading dot, runs
// only through the jobs that extend it.
func (r *gitlabReader) jobs() []*models.Job {
	jobs := r.included
	for _, name := range slices.Sorted(maps.Keys(r.definitions)) {
		if strings.HasPrefix(name, ".") {
			continue
		}
		if job, ok := r.resolve(name, 0); ok {
			jobs = append(jobs, gitlabJobFrom(name, job))
		}
	}
	return jobs
}

// resolve merges a job over the jobs it extends, in order, each key replaced
// whole, as GitLab does for anything but a mapping.
func (r *gitlabReader) resolve(name string, depth int) (gitlabJob, bool) {
	var job gitlabJob
	node, ok := r.definitions[name]
	if !ok || depth > gitlabExtendsDepth {
		return job, false
	}
	if node = resolve(node); node.Kind != yaml.MappingNode || node.Decode(&job) != nil {
		return job, false
	}
	var merged gitlabJob
	for _, parent := range extended(&job.Extends) {
		if base, ok := r.resolve(parent, depth+1); ok {
			merged = overlay(merged, base)
		}
	}
	return overlay(merged, job), true
}

func extended(node *yaml.Node) []string {
	node = resolve(node)
	entries := []*yaml.Node{node}
	if node.Kind == yaml.SequenceNode {
		entries = node.Content
	}
	var names []string
	for _, entry := range entries {
		if entry = resolve(entry); entry.Kind == yaml.ScalarNode && entry.Value != "" {
			names = append(names, entry.Value)
		}
	}
	return names
}

func overlay(base, job gitlabJob) gitlabJob {
	for _, field := range []struct{ to, from *yaml.Node }{
		{&base.BeforeScript, &job.BeforeScript}, {&base.Script, &job.Script}, {&base.AfterScript, &job.AfterScript},
		{&base.Image, &job.Image}, {&base.AllowFailure, &job.AllowFailure}, {&base.When, &job.When},
	} {
		if field.from.Kind != 0 {
			*field.to = *field.from
		}
	}
	return base
}

func gitlabJobFrom(name string, job gitlabJob) *models.Job {
	// Script blocks carry the same signal `uses:` does for GitHub Actions.
	var script []string
	for _, block := range []*yaml.Node{&job.BeforeScript, &job.Script, &job.AfterScript} {
		script = append(script, scriptLines(block)...)
	}
	record := &models.Job{
		Name:            name,
		Package:         imageName(&job.Image),
		Run:             strings.Join(script, "\n"),
		ContinueOnError: scalarIs(&job.AllowFailure, "true"),
	}
	// A manual job with allow_failure: false holds the pipeline until it runs
	// and passes; whether that blocks a merge is a project setting.
	if scalarIs(&job.When, "manual") {
		if scalarIs(&job.AllowFailure, "false") {
			record.If = "when: manual"
		} else {
			record.Manual = true
		}
	}
	return record
}

// includes reads include:, which takes a path, a list, or mappings. A local
// file is read as more of this configuration. A GitLab template or CI/CD
// component cannot be read, so it is recorded by name for the rules to match;
// a project or remote include cannot be read at all and is skipped.
func (r *gitlabReader) includes(node *yaml.Node) {
	node = resolve(node)
	entries := []*yaml.Node{node}
	if node.Kind == yaml.SequenceNode {
		entries = node.Content
	}

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
			r.local(include.Local)
		case include.Template != "":
			r.included = append(r.included, &models.Job{Name: "include", Package: include.Template})
		case include.Component != "":
			r.included = append(r.included, &models.Job{Name: "include", Package: include.Component})
		}
	}
}

// local reads files included from this repository. GitLab resolves them from
// the repository root and allows wildcards; a remote URL or a path built from a
// variable cannot be read here.
func (r *gitlabReader) local(pattern string) {
	if strings.Contains(pattern, "://") || strings.Contains(pattern, "$") || r.depth >= gitlabIncludeDepth {
		return
	}
	matches, _ := filepath.Glob(filepath.Join(r.root, pattern))

	r.depth++
	defer func() { r.depth-- }()
	for _, path := range matches {
		if rel, err := filepath.Rel(r.root, path); err != nil || strings.HasPrefix(rel, "..") {
			continue
		}
		if config, err := readGitlabConfig(r.root, path); err == nil {
			r.collect(config)
		}
	}
}

func NewGitlabParser() Parser {
	return &GitlabParser{}
}
