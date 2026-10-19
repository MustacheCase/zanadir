package parser

import (
	"path/filepath"
	"strings"

	"github.com/MustacheCase/zanadir/models"
	"github.com/MustacheCase/zanadir/utils"
	"gopkg.in/yaml.v3"
)

type CircleCIParser struct{}

// circleCIDef holds only what is read. Fields like docker: and workflows: take
// shapes a fixed type cannot, and decoding them anyway aborted whole scans.
type circleCIDef struct {
	Orbs      map[string]yaml.Node      `yaml:"orbs"`
	Jobs      map[string]circleCIJobDef `yaml:"jobs"`
	Workflows yaml.Node                 `yaml:"workflows"`
}

type circleCIJobDef struct {
	Steps []interface{} `yaml:"steps"`
}

func circleCIConfigs(root string) []string {
	return utils.YAMLFiles(filepath.Join(root, ".circleci"))
}

func (c *CircleCIParser) Exists(root string) bool {
	return len(circleCIConfigs(root)) > 0
}

func (c *CircleCIParser) Parse(root string) ([]*models.Artifact, error) {
	var artifacts []*models.Artifact
	for _, path := range circleCIConfigs(root) {
		artifact, err := c.parseCircleCIWorkflow(path)
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, artifact)
	}
	return artifacts, nil
}

func (c *CircleCIParser) parseCircleCIWorkflow(filePath string) (*models.Artifact, error) {
	var wf circleCIDef
	if err := readYAML(filePath, &wf); err != nil {
		return unreadable(filePath, err), nil
	}

	// Only what a workflow runs counts: a job no workflow lists never runs,
	// and an orb is used through a workflow job or a running job's step.
	// Without workflows, every job and orb is taken to run, as before.
	running, ok := workflowJobs(&wf.Workflows)
	used := map[string]bool{}
	for name := range running {
		used[name] = true
	}
	for jobName, job := range wf.Jobs {
		if ok && !running[jobName] {
			continue
		}
		for _, step := range stepNames(job.Steps) {
			used[step] = true
		}
	}

	var jobs []*models.Job
	for name, orb := range wf.Orbs {
		// An orb defined inline is a mapping, not a reference to a published one.
		if ref := resolve(&orb); ref.Kind == yaml.ScalarNode && ref.Value != "" && (!ok || usesOrb(used, name)) {
			pkg, version, _ := strings.Cut(ref.Value, "@")
			jobs = append(jobs, &models.Job{Name: name, Package: pkg, Version: version})
		}
	}

	// Orbs are only half the picture; jobs also invoke tools from run steps.
	for jobName, job := range wf.Jobs {
		if commands := extractRunCommands(job.Steps); len(commands) > 0 && (!ok || running[jobName]) {
			jobs = append(jobs, &models.Job{Name: jobName, Run: strings.Join(commands, "\n")})
		}
	}

	return &models.Artifact{
		Name:     "CircleCI Workflow Orbs",
		Jobs:     jobs,
		Location: filePath,
	}, nil
}

// workflowJobs names the jobs the workflows run, and reports whether there
// are workflows at all. A job is listed by name or as a one-key mapping.
func workflowJobs(node *yaml.Node) (map[string]bool, bool) {
	node = resolve(node)
	if node.Kind != yaml.MappingNode {
		return nil, false
	}
	running := map[string]bool{}
	for i := 1; i < len(node.Content); i += 2 {
		workflow := resolve(node.Content[i])
		for j := 0; workflow.Kind == yaml.MappingNode && j+1 < len(workflow.Content); j += 2 {
			if list := resolve(workflow.Content[j+1]); workflow.Content[j].Value == "jobs" && list.Kind == yaml.SequenceNode {
				for _, item := range list.Content {
					item = resolve(item)
					if item.Kind == yaml.MappingNode && len(item.Content) > 0 {
						item = item.Content[0]
					}
					running[item.Value] = true
				}
			}
		}
	}
	return running, true
}

// stepNames are the steps a job calls by name, such as checkout or an orb's
// snyk/scan, in either the plain or the mapping form, including those under
// a when or unless step.
func stepNames(steps []interface{}) []string {
	var names []string
	for _, step := range steps {
		switch v := step.(type) {
		case string:
			names = append(names, v)
		case map[string]interface{}:
			for name, body := range v {
				names = append(names, name)
				if conditional, ok := body.(map[string]interface{}); ok && (name == "when" || name == "unless") {
					nested, _ := conditional["steps"].([]interface{})
					names = append(names, stepNames(nested)...)
				}
			}
		}
	}
	return names
}

func usesOrb(used map[string]bool, orb string) bool {
	for name := range used {
		if strings.HasPrefix(name, orb+"/") {
			return true
		}
	}
	return false
}

// extractRunCommands pulls shell command text out of a CircleCI steps list.
// A run step is either a plain command or a mapping with a "command" key.
func extractRunCommands(steps []interface{}) []string {
	var commands []string
	for _, step := range steps {
		stepMap, ok := step.(map[string]interface{})
		if !ok {
			continue
		}
		run, ok := stepMap["run"]
		if !ok {
			continue
		}
		switch v := run.(type) {
		case string:
			commands = append(commands, v)
		case map[string]interface{}:
			if command, ok := v["command"].(string); ok {
				commands = append(commands, command)
			}
		}
	}
	return commands
}

func NewCircleCIParser() Parser {
	return &CircleCIParser{}
}
