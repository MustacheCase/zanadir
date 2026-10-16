package parser

import (
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"github.com/MustacheCase/zanadir/models"
	"github.com/MustacheCase/zanadir/utils"
	"gopkg.in/yaml.v3"
)

const bitbucketFile = "bitbucket-pipelines.yml"

type BitbucketParser struct{}

type bitbucketConfig struct {
	Image     yaml.Node `yaml:"image"`
	Pipelines struct {
		Default      yaml.Node            `yaml:"default"`
		Branches     map[string]yaml.Node `yaml:"branches"`
		PullRequests map[string]yaml.Node `yaml:"pull-requests"`
		Tags         map[string]yaml.Node `yaml:"tags"`
		Bookmarks    map[string]yaml.Node `yaml:"bookmarks"`
		Custom       map[string]yaml.Node `yaml:"custom"`
	} `yaml:"pipelines"`
}

type bitbucketItem struct {
	Step     *bitbucketStep  `yaml:"step"`
	Parallel yaml.Node       `yaml:"parallel"`
	Stage    *bitbucketStage `yaml:"stage"`
}

type bitbucketStep struct {
	Name        string      `yaml:"name"`
	Trigger     string      `yaml:"trigger"`
	Image       yaml.Node   `yaml:"image"`
	Script      []yaml.Node `yaml:"script"`
	AfterScript []yaml.Node `yaml:"after-script"`
}

type bitbucketStage struct {
	Trigger string      `yaml:"trigger"`
	Steps   []yaml.Node `yaml:"steps"`
}

type bitbucketPipe struct {
	Pipe      string               `yaml:"pipe"`
	Variables map[string]yaml.Node `yaml:"variables"`
}

func (b *BitbucketParser) Exists(root string) bool {
	return utils.IsFile(filepath.Join(root, bitbucketFile))
}

// Parse returns one artifact per pipeline, since each section of the file runs
// on a different trigger and the trigger is what grading reads.
func (b *BitbucketParser) Parse(root string) ([]*models.Artifact, error) {
	path := filepath.Join(root, bitbucketFile)
	var config bitbucketConfig
	if err := utils.ReadYAML(path, &config); err != nil {
		return nil, err
	}

	defaultImage := imageName(&config.Image)
	var artifacts []*models.Artifact
	add := func(node *yaml.Node, triggers []models.Trigger) {
		jobs := bitbucketJobs(node, defaultImage)
		if len(jobs) == 0 {
			return
		}
		artifacts = append(artifacts, &models.Artifact{
			Name:     "Bitbucket Pipelines",
			Triggers: triggers,
			Jobs:     jobs,
			Location: path,
		})
	}

	p := config.Pipelines
	add(&p.Default, nil)
	sections := []struct {
		pipelines map[string]yaml.Node
		triggers  func(key string) []models.Trigger
	}{
		{p.Branches, branchTriggers},
		{p.Bookmarks, branchTriggers},
		{p.PullRequests, on("pull_request")},
		{p.Tags, on("tag")},
		{p.Custom, on("workflow_dispatch")},
	}
	for _, section := range sections {
		for _, key := range slices.Sorted(maps.Keys(section.pipelines)) {
			node := section.pipelines[key]
			add(&node, section.triggers(key))
		}
	}
	return artifacts, nil
}

// on gives every pipeline in a section the same trigger, whatever its key.
func on(event string) func(string) []models.Trigger {
	return func(string) []models.Trigger { return []models.Trigger{{Event: event}} }
}

// branchTriggers records a push only for a single named branch, which runs
// after a merge. The default pipeline and wildcard patterns also run on a pull
// request's source branch, where whether they block the merge is a repository
// setting the file does not hold, so they are left without a trigger and grade
// unknown, as GitLab branch pipelines do.
func branchTriggers(pattern string) []models.Trigger {
	if strings.ContainsAny(pattern, "*?{[") {
		return nil
	}
	return []models.Trigger{{Event: "push", Branches: []string{pattern}}}
}

// bitbucketJobs reads a pipeline's list of steps. An entry that cannot be read,
// or a pipeline that is an import from another repository, yields nothing
// rather than failing the scan.
func bitbucketJobs(node *yaml.Node, defaultImage string) []*models.Job {
	node = resolve(node)
	if node == nil || node.Kind != yaml.SequenceNode {
		return nil
	}
	var jobs []*models.Job
	for _, entry := range node.Content {
		jobs = append(jobs, bitbucketItemJobs(entry, defaultImage, false)...)
	}
	return jobs
}

func bitbucketItemJobs(node *yaml.Node, defaultImage string, manual bool) []*models.Job {
	var item bitbucketItem
	if resolve(node).Decode(&item) != nil {
		return nil
	}

	var jobs []*models.Job
	if item.Step != nil {
		jobs = append(jobs, bitbucketStepJobs(item.Step, defaultImage, manual)...)
	}
	if item.Stage != nil {
		stageManual := manual || strings.EqualFold(item.Stage.Trigger, "manual")
		for i := range item.Stage.Steps {
			jobs = append(jobs, bitbucketItemJobs(&item.Stage.Steps[i], defaultImage, stageManual)...)
		}
	}
	if parallel := resolve(&item.Parallel); parallel != nil {
		steps := parallel.Content
		if parallel.Kind == yaml.MappingNode {
			var block struct {
				Steps []yaml.Node `yaml:"steps"`
			}
			if parallel.Decode(&block) != nil {
				return jobs
			}
			steps = make([]*yaml.Node, len(block.Steps))
			for i := range block.Steps {
				steps[i] = &block.Steps[i]
			}
		}
		for _, step := range steps {
			jobs = append(jobs, bitbucketItemJobs(step, defaultImage, manual)...)
		}
	}
	return jobs
}

// bitbucketStepJobs splits a step into its script, each pipe it calls, and its
// after-script. A failing after-script does not fail the step, so anything
// there is recorded as unable to fail a build.
func bitbucketStepJobs(step *bitbucketStep, defaultImage string, manual bool) []*models.Job {
	manual = manual || strings.EqualFold(step.Trigger, "manual")
	image := imageName(&step.Image)
	if image == "" {
		image = defaultImage
	}

	run, pipes := bitbucketScript(step.Script)
	var jobs []*models.Job
	if run != "" || image != "" {
		jobs = append(jobs, &models.Job{Name: step.Name, Package: image, Run: run, Manual: manual})
	}
	for _, pipe := range pipes {
		jobs = append(jobs, bitbucketPipeJob(step.Name, pipe, manual, false))
	}

	afterRun, afterPipes := bitbucketScript(step.AfterScript)
	if afterRun != "" {
		jobs = append(jobs, &models.Job{Name: step.Name, Run: afterRun, Manual: manual, ContinueOnError: true})
	}
	for _, pipe := range afterPipes {
		jobs = append(jobs, bitbucketPipeJob(step.Name, pipe, manual, true))
	}
	return jobs
}

func bitbucketScript(items []yaml.Node) (string, []bitbucketPipe) {
	var lines []string
	var pipes []bitbucketPipe
	for i := range items {
		item := resolve(&items[i])
		switch item.Kind {
		case yaml.ScalarNode:
			if item.Value != "" {
				lines = append(lines, item.Value)
			}
		case yaml.MappingNode:
			var pipe bitbucketPipe
			if item.Decode(&pipe) == nil && pipe.Pipe != "" {
				pipes = append(pipes, pipe)
			}
		}
	}
	return strings.Join(lines, "\n"), pipes
}

func bitbucketPipeJob(stepName string, pipe bitbucketPipe, manual, afterScript bool) *models.Job {
	ref := strings.TrimPrefix(pipe.Pipe, "docker://")
	name, version := ref, ""
	if i := strings.LastIndex(ref, ":"); i > strings.LastIndex(ref, "/") {
		name, version = ref[:i], ref[i+1:]
	}

	return &models.Job{
		Name:            stepName,
		Package:         name,
		Version:         version,
		With:            scalars(pipe.Variables),
		Manual:          manual,
		ContinueOnError: afterScript,
	}
}

func NewBitbucketParser() Parser {
	return &BitbucketParser{}
}
