package parser

import (
	"fmt"
	"os"
	"path/filepath"
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
	Uses            string     `yaml:"uses"`
	If              string     `yaml:"if"`
	ContinueOnError bool       `yaml:"continue-on-error"`
	Needs           stringList `yaml:"needs"`
	Steps           []stepDef  `yaml:"steps"`
}

type stepDef struct {
	Name            string         `yaml:"name"`
	Uses            string         `yaml:"uses"`
	Run             string         `yaml:"run"`
	If              string         `yaml:"if"`
	ContinueOnError bool           `yaml:"continue-on-error"`
	With            map[string]any `yaml:"with"`
}

// stringList accepts either a scalar or a sequence, which "needs" and the
// branch filters both allow.
type stringList []string

func (s *stringList) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		var one string
		if err := node.Decode(&one); err != nil {
			return err
		}
		*s = stringList{one}
		return nil
	case yaml.SequenceNode:
		var many []string
		if err := node.Decode(&many); err != nil {
			return err
		}
		*s = many
		return nil
	default:
		return nil
	}
}

// triggerFilter is the map form of one event, e.g. "push: {branches: [main]}".
type triggerFilter struct {
	Branches stringList `yaml:"branches"`
}

// parseTriggers reads the three shapes "on" can take: a single event, a list
// of events, or a map of events to their filters.
func parseTriggers(node yaml.Node) []models.Trigger {
	switch node.Kind {
	case yaml.ScalarNode:
		var event string
		if err := node.Decode(&event); err != nil || event == "" {
			return nil
		}
		return []models.Trigger{{Event: event}}

	case yaml.SequenceNode:
		var events []string
		if err := node.Decode(&events); err != nil {
			return nil
		}
		triggers := make([]models.Trigger, 0, len(events))
		for _, event := range events {
			triggers = append(triggers, models.Trigger{Event: event})
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
			record := &models.Job{
				Name: jobName,
				// Either level means a failure here cannot fail the build.
				ContinueOnError: step.ContinueOnError || job.ContinueOnError,
				If:              step.If,
				JobIf:           job.If,
				Needs:           job.Needs,
				With:            stepInputs(step.With),
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
				jobs = append(jobs, record)
			}
		}
	}

	return &models.Artifact{
		Name:     wf.Name,
		Triggers: parseTriggers(wf.On),
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
