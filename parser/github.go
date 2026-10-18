package parser

import (
	"cmp"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/MustacheCase/zanadir/models"
	"github.com/MustacheCase/zanadir/utils"
	"gopkg.in/yaml.v3"
)

type GithubParser struct{}

type workflowDef struct {
	Name string
	// yaml.v3 keeps "on" a string key rather than folding it to a boolean the
	// way YAML 1.1 would, so the field needs no alias.
	On   yaml.Node                 `yaml:"on"`
	Jobs map[string]workflowJobDef `yaml:"jobs"`
}

type workflowJobDef struct {
	Name            string          `yaml:"name"`
	Uses            string          `yaml:"uses"`
	If              string          `yaml:"if"`
	ContinueOnError continueOnError `yaml:"continue-on-error"`
	Needs           stringList      `yaml:"needs"`
	Steps           []stepDef       `yaml:"steps"`
}

type stepDef struct {
	Name            string          `yaml:"name"`
	Uses            string          `yaml:"uses"`
	Run             string          `yaml:"run"`
	If              string          `yaml:"if"`
	ContinueOnError continueOnError `yaml:"continue-on-error"`
	With            map[string]any  `yaml:"with"`
}

// continueOnError is true, false, or an expression such as
// ${{ matrix.experimental }} that decides at run time. Decoding it as a bool
// aborted the scan of every repository that used an expression.
type continueOnError struct {
	always     bool
	expression string
}

func (c *continueOnError) UnmarshalYAML(node *yaml.Node) error {
	switch {
	case scalarIs(node, "true"):
		c.always = true
	case scalarIs(node, "false"):
	default:
		c.expression = node.Value
	}
	return nil
}

// condition is the expression as a condition grading cannot read, so the
// control grades unknown rather than either way.
func (c continueOnError) condition() string {
	if c.expression == "" {
		return ""
	}
	return "continue-on-error: " + c.expression
}

// triggerFilter is the map form of one event, e.g. "push: {branches: [main]}".
type triggerFilter struct {
	Branches stringList `yaml:"branches"`
}

// parseTriggers reads the three shapes "on" can take: a single event, a list
// of events, or a map of events to their filters.
func parseTriggers(node *yaml.Node) []models.Trigger {
	switch node.Kind {
	case yaml.ScalarNode, yaml.SequenceNode:
		events := decodeStrings(node)
		triggers := make([]models.Trigger, 0, len(events))
		for _, event := range events {
			triggers = append(triggers, models.Trigger{Event: event})
		}
		if len(triggers) == 0 {
			return nil
		}
		return triggers

	case yaml.MappingNode:
		// Content alternates key, value. Decoding into a map would lose the
		// order events were written in.
		triggers := make([]models.Trigger, 0, len(node.Content)/2)
		for i := 0; i+1 < len(node.Content); i += 2 {
			event := node.Content[i].Value
			if event == "" {
				continue
			}
			// A null value is "push:" with no filters; anything that is not a
			// mapping, such as schedule's list of crons, carries no branches.
			var filter triggerFilter
			if node.Content[i+1].Kind == yaml.MappingNode {
				_ = node.Content[i+1].Decode(&filter)
			}
			triggers = append(triggers, models.Trigger{Event: event, Branches: filter.Branches})
		}
		if len(triggers) == 0 {
			return nil
		}
		return triggers
	}
	return nil
}

// stepInputs flattens "with" to strings. Values are written as booleans and
// numbers as often as strings, and every consumer wants to match text.
func stepInputs(with map[string]any) map[string]string {
	if len(with) == 0 {
		return nil
	}
	inputs := make(map[string]string, len(with))
	for key, value := range with {
		inputs[key] = fmt.Sprint(value)
	}
	return inputs
}

// Only files directly in the directory are workflows; GitHub ignores the rest.
func githubWorkflows(root string) []string {
	return utils.YAMLFiles(filepath.Join(root, ".github", "workflows"))
}

func (g *GithubParser) Exists(root string) bool {
	return len(githubWorkflows(root)) > 0
}

func (g *GithubParser) Parse(root string) ([]*models.Artifact, error) {
	var artifacts []*models.Artifact
	for _, path := range githubWorkflows(root) {
		artifact, err := g.parseGithubWorkflow(path)
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, artifact)
	}
	inheritCallers(artifacts)
	return artifacts, nil
}

const localWorkflows = "./.github/workflows/"

// inheritCallers grades a local reusable workflow by what calls it: on:
// workflow_call says nothing about when it runs, its callers' triggers do.
// A chain of calls settles within as many passes as there are workflows.
// The callers' conditions carry over too, and with a single caller the
// called jobs report their checks as "caller / job", as GitHub names them.
func inheritCallers(artifacts []*models.Artifact) {
	byFile := map[string]*models.Artifact{}
	for _, a := range artifacts {
		byFile[localWorkflows+filepath.Base(a.Location)] = a
	}
	callers := map[*models.Artifact][]*models.Job{}
	for range artifacts {
		changed := false
		for _, caller := range artifacts {
			for _, job := range caller.Jobs {
				called := byFile[job.Package]
				if called == nil || called == caller || !called.HasTrigger("workflow_call") {
					continue
				}
				if !slices.Contains(callers[called], job) {
					callers[called] = append(callers[called], job)
				}
				for _, trigger := range caller.Triggers {
					if !called.HasTrigger(trigger.Event) {
						called.Triggers = append(called.Triggers, trigger)
						changed = true
					}
				}
			}
		}
		if !changed {
			break
		}
	}
	for called, jobs := range callers {
		allMayFail := !slices.ContainsFunc(jobs, func(j *models.Job) bool { return !j.ContinueOnError })
		for _, job := range called.Jobs {
			job.ContinueOnError = job.ContinueOnError || allMayFail
			for _, caller := range jobs {
				job.JobIf = joinConditions(job.JobIf, caller.JobIf)
			}
			if len(jobs) == 1 {
				job.Check = jobs[0].Check + " / " + job.Check
			}
		}
	}
}

func (g *GithubParser) parseGithubWorkflow(filePath string) (*models.Artifact, error) {
	var wf workflowDef
	if err := readYAML(filePath, &wf); err != nil {
		return unreadable(filePath, err)
	}

	var jobs []*models.Job
	for jobName, job := range wf.Jobs {
		// A job calling a reusable workflow has no steps but still reports a
		// check, as "name / called job".
		if job.Uses != "" {
			pkg, version := parseStepUsageStatement(job.Uses)
			jobs = append(jobs, &models.Job{Name: jobName, Check: cmp.Or(job.Name, jobName), Needs: job.Needs,
				Package: pkg, Version: version,
				ContinueOnError: job.ContinueOnError.always, JobIf: joinConditions(job.If, job.ContinueOnError.condition())})
		}
		for _, step := range job.Steps {
			record := &models.Job{
				Name: jobName,
				// Either level means a failure here cannot fail the build.
				ContinueOnError: step.ContinueOnError.always || job.ContinueOnError.always,
				If:              joinConditions(step.If, step.ContinueOnError.condition()),
				JobIf:           joinConditions(job.If, job.ContinueOnError.condition()),
				Needs:           job.Needs,
				With:            stepInputs(step.With),
				Check:           cmp.Or(job.Name, jobName),
			}

			// Tools invoked directly, e.g. "trivy fs .", only appear in Run.
			if step.Run != "" {
				record.Run = step.Run
				jobs = append(jobs, record)
				continue
			}
			if step.Uses == "" {
				continue
			}
			pkgName, version := parseStepUsageStatement(step.Uses)
			if pkgName != "" {
				record.Package, record.Version = pkgName, version
				// Wrapper actions such as a retry or a problem matcher take the
				// command they run as an input.
				record.Run = cmp.Or(record.With["run"], record.With["command"])
				if pkgName == "goreleaser/goreleaser-action" {
					record.Run = strings.TrimSpace("goreleaser " + record.With["args"])
				}
				jobs = append(jobs, record)
			}
		}
	}

	return &models.Artifact{
		Name:     wf.Name,
		Triggers: parseTriggers(&wf.On),
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
