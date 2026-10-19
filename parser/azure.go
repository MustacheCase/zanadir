package parser

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/MustacheCase/zanadir/internal/repofs"
	"github.com/MustacheCase/zanadir/models"
	"github.com/MustacheCase/zanadir/utils"
	"gopkg.in/yaml.v3"
)

var azureFiles = []string{"azure-pipelines.yml", "azure-pipelines.yaml"}

// Templates nest; this bounds a chain that includes itself.
const azureTemplateDepth = 8

type AzureParser struct{}

type azureDoc struct {
	Trigger   yaml.Node   `yaml:"trigger"`
	PR        yaml.Node   `yaml:"pr"`
	Schedules []yaml.Node `yaml:"schedules"`
	Extends   struct {
		Template   string    `yaml:"template"`
		Parameters yaml.Node `yaml:"parameters"`
	} `yaml:"extends"`
	Stages []yaml.Node `yaml:"stages"`
	Jobs   []yaml.Node `yaml:"jobs"`
	Steps  []yaml.Node `yaml:"steps"`
}

type azureStage struct {
	Template   string      `yaml:"template"`
	Parameters yaml.Node   `yaml:"parameters"`
	Condition  string      `yaml:"condition"`
	Jobs       []yaml.Node `yaml:"jobs"`
}

type azureJob struct {
	Job             string      `yaml:"job"`
	Deployment      string      `yaml:"deployment"`
	Template        string      `yaml:"template"`
	Parameters      yaml.Node   `yaml:"parameters"`
	Condition       string      `yaml:"condition"`
	ContinueOnError yaml.Node   `yaml:"continueOnError"`
	Steps           []yaml.Node `yaml:"steps"`
	Strategy        yaml.Node   `yaml:"strategy"`
}

type azureStep struct {
	Template        string               `yaml:"template"`
	Parameters      yaml.Node            `yaml:"parameters"`
	Task            string               `yaml:"task"`
	Script          string               `yaml:"script"`
	Bash            string               `yaml:"bash"`
	Pwsh            string               `yaml:"pwsh"`
	PowerShell      string               `yaml:"powershell"`
	Condition       string               `yaml:"condition"`
	ContinueOnError yaml.Node            `yaml:"continueOnError"`
	Enabled         yaml.Node            `yaml:"enabled"`
	Inputs          map[string]yaml.Node `yaml:"inputs"`
}

// azureContext is what a step inherits from the job and stage around it.
type azureContext struct {
	job             string
	condition       string
	continueOnError bool
}

type azureReader struct {
	root  string
	depth int
}

func (a *AzureParser) Exists(root string) bool {
	return azurePipelineFile(root) != ""
}

func azurePipelineFile(root string) string {
	if name := utils.FirstFile(root, azureFiles...); name != "" {
		return filepath.Join(root, name)
	}
	return ""
}

func (a *AzureParser) Parse(root string) ([]*models.Artifact, error) {
	path := azurePipelineFile(root)
	var doc azureDoc
	if err := readYAML(root, path, &doc); err != nil {
		return skippedFile(path, err)
	}

	r := &azureReader{root: root}
	return []*models.Artifact{{
		Name:     "Azure Pipelines",
		Triggers: azureTriggers(&doc),
		Jobs:     r.document(&doc, filepath.Dir(path), azureContext{}),
		Location: path,
	}}, nil
}

// azureTriggers records a pull request only when pr: names branches. Without
// it, an Azure Repos pull request is validated by a branch policy outside the
// repository, so the pipeline is left without a trigger and grades unknown.
// A pipeline that has opted out of both and only runs on a schedule cannot
// gate a change.
func azureTriggers(doc *azureDoc) []models.Trigger {
	if azureSet(&doc.PR) {
		return []models.Trigger{{Event: "pull_request"}}
	}
	if scalarIs(&doc.Trigger, "none") && scalarIs(&doc.PR, "none") && len(doc.Schedules) > 0 {
		return []models.Trigger{{Event: "schedule"}}
	}
	return nil
}

func azureSet(node *yaml.Node) bool {
	node = resolve(node)
	switch node.Kind {
	case yaml.ScalarNode:
		return node.Value != "" && !strings.EqualFold(node.Value, "none")
	case yaml.SequenceNode, yaml.MappingNode:
		return len(node.Content) > 0
	}
	return false
}

func (r *azureReader) document(doc *azureDoc, dir string, ctx azureContext) []*models.Job {
	var jobs []*models.Job
	if doc.Extends.Template != "" {
		jobs = append(jobs, r.template(doc.Extends.Template, &doc.Extends.Parameters, dir, ctx)...)
	}
	for i := range doc.Stages {
		jobs = append(jobs, r.stage(&doc.Stages[i], dir, ctx)...)
	}
	for i := range doc.Jobs {
		jobs = append(jobs, r.job(&doc.Jobs[i], dir, ctx)...)
	}
	for i := range doc.Steps {
		jobs = append(jobs, r.step(&doc.Steps[i], dir, ctx)...)
	}
	return jobs
}

// template reads a template from this repository. One in another repository
// (path@alias) or built from an expression cannot be read, and yields nothing.
// Its parameters are substituted from the call, or their defaults.
func (r *azureReader) template(ref string, given *yaml.Node, dir string, ctx azureContext) []*models.Job {
	ref = strings.TrimSuffix(ref, "@self")
	if ref == "" || strings.Contains(ref, "@") || strings.Contains(ref, "${{") || r.depth >= azureTemplateDepth {
		return nil
	}
	path := filepath.Join(dir, ref)
	if strings.HasPrefix(ref, "/") {
		path = filepath.Join(r.root, ref)
	}
	if rel, err := filepath.Rel(r.root, path); err != nil || strings.HasPrefix(rel, "..") {
		return nil
	}

	data, err := repofs.ReadFile(r.root, path)
	if err != nil {
		return nil
	}
	var declared struct {
		Parameters yaml.Node `yaml:"parameters"`
	}
	_ = decodeYAML(data, &declared)
	// A value can break the YAML it is put into; the template is then read
	// as written.
	var doc azureDoc
	if decodeYAML([]byte(substitute(string(data), parameterValues(&declared.Parameters, given))), &doc) != nil &&
		decodeYAML(data, &doc) != nil {
		return nil
	}
	r.depth++
	defer func() { r.depth-- }()
	return r.document(&doc, filepath.Dir(path), ctx)
}

func (r *azureReader) stage(node *yaml.Node, dir string, ctx azureContext) []*models.Job {
	var stage azureStage
	if resolve(node).Decode(&stage) != nil {
		return nil
	}
	if stage.Template != "" {
		return r.template(stage.Template, &stage.Parameters, dir, ctx)
	}
	ctx.condition = joinConditions(ctx.condition, stage.Condition)
	var jobs []*models.Job
	for i := range stage.Jobs {
		jobs = append(jobs, r.job(&stage.Jobs[i], dir, ctx)...)
	}
	return jobs
}

func (r *azureReader) job(node *yaml.Node, dir string, ctx azureContext) []*models.Job {
	var job azureJob
	if resolve(node).Decode(&job) != nil {
		return nil
	}
	if job.Template != "" {
		return r.template(job.Template, &job.Parameters, dir, ctx)
	}

	ctx.job = job.Job
	if ctx.job == "" {
		ctx.job = job.Deployment
	}
	continues, unread := runtimeFlag("continueOnError", &job.ContinueOnError)
	ctx.condition = joinConditions(joinConditions(ctx.condition, job.Condition), unread)
	ctx.continueOnError = ctx.continueOnError || continues

	steps := make([]*yaml.Node, 0, len(job.Steps))
	for i := range job.Steps {
		steps = append(steps, &job.Steps[i])
	}
	// A deployment job keeps its steps inside its strategy's lifecycle hooks.
	steps = append(steps, nestedSteps(&job.Strategy)...)

	var jobs []*models.Job
	for _, step := range steps {
		jobs = append(jobs, r.step(step, dir, ctx)...)
	}
	return jobs
}

func nestedSteps(node *yaml.Node) []*yaml.Node {
	node = resolve(node)
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	var steps []*yaml.Node
	for i := 0; i+1 < len(node.Content); i += 2 {
		key, value := node.Content[i], resolve(node.Content[i+1])
		if key.Value == "steps" && value.Kind == yaml.SequenceNode {
			steps = append(steps, value.Content...)
			continue
		}
		steps = append(steps, nestedSteps(value)...)
	}
	return steps
}

func (r *azureReader) step(node *yaml.Node, dir string, ctx azureContext) []*models.Job {
	var step azureStep
	if resolve(node).Decode(&step) != nil {
		return nil
	}
	if step.Template != "" {
		return r.template(step.Template, &step.Parameters, dir, ctx)
	}
	// A disabled step never runs, so it protects nothing and is not coverage.
	if scalarIs(&step.Enabled, "false") {
		return nil
	}
	_, disabledBy := runtimeFlag("enabled", &step.Enabled)
	continues, unread := runtimeFlag("continueOnError", &step.ContinueOnError)

	record := &models.Job{
		Name:            ctx.job,
		ContinueOnError: ctx.continueOnError || continues,
		If:              joinConditions(joinConditions(azureCondition(step.Condition), disabledBy), unread),
		JobIf:           ctx.condition,
	}
	for _, run := range []string{step.Script, step.Bash, step.Pwsh, step.PowerShell} {
		if run != "" {
			record.Run = run
			return []*models.Job{record}
		}
	}
	if step.Task == "" {
		return nil
	}

	record.Package, record.Version, _ = strings.Cut(step.Task, "@")
	record.With = scalars(step.Inputs)
	if with := record.With; strings.EqualFold(record.Package, "trivy") {
		fallback := "filesystem"
		if with["image"] != "" {
			fallback = "image"
		}
		record.Run = trivyCommand(scanType(with["type"], fallback), with["scanners"], with["exitCode"], "", with["options"])
	}
	return []*models.Job{record}
}

// runtimeFlag reads a boolean that may be an expression decided at run time,
// which is returned as a condition grading cannot read, as on GitHub.
func runtimeFlag(name string, node *yaml.Node) (bool, string) {
	node = resolve(node)
	switch {
	case scalarIs(node, "true"):
		return true, ""
	case node.Kind != yaml.ScalarNode || node.Value == "" || scalarIs(node, "false"):
		return false, ""
	}
	return false, name + ": " + node.Value
}

var (
	azureExpression = regexp.MustCompile(`\$\{\{(.*?)\}\}`)
	parameterRef    = regexp.MustCompile(`\bparameters\.([A-Za-z_][\w-]*)`)
	negation        = regexp.MustCompile(`(?i)^not\(\s*(true|false)\s*\)$`)
)

// parameterValues are a template's scalar parameters: those the call gives,
// else the defaults it declares, as a list of name and default or a mapping.
func parameterValues(declared, given *yaml.Node) map[string]string {
	values := map[string]string{}
	declared = resolve(declared)
	if declared.Kind == yaml.SequenceNode {
		for _, item := range declared.Content {
			var parameter struct {
				Name    string    `yaml:"name"`
				Default yaml.Node `yaml:"default"`
			}
			if resolve(item).Decode(&parameter) == nil && parameter.Default.Kind == yaml.ScalarNode {
				values[parameter.Name] = parameter.Default.Value
			}
		}
	}
	for _, node := range []*yaml.Node{declared, resolve(given)} {
		for i := 0; node.Kind == yaml.MappingNode && i+1 < len(node.Content); i += 2 {
			if value := resolve(node.Content[i+1]); value.Kind == yaml.ScalarNode {
				values[node.Content[i].Value] = value.Value
			}
		}
	}
	return values
}

// substitute puts known parameters into each ${{ }} expression. One that is
// then a literal, or not() of one, becomes its value; the rest stay
// expressions.
func substitute(text string, values map[string]string) string {
	return azureExpression.ReplaceAllStringFunc(text, func(expression string) string {
		known := true
		body := parameterRef.ReplaceAllStringFunc(strings.TrimSpace(azureExpression.FindStringSubmatch(expression)[1]), func(ref string) string {
			value, ok := values[strings.TrimPrefix(ref, "parameters.")]
			known = known && ok
			return value
		})
		switch match := negation.FindStringSubmatch(body); {
		case !known:
			return expression
		case match != nil:
			return strconv.FormatBool(!strings.EqualFold(match[1], "true"))
		case !strings.ContainsAny(body, "(),"):
			return body
		}
		return "${{ " + body + " }}"
	})
}

// azureCondition drops succeeded(), the default every step has anyway. Any
// other condition is kept, and grading never interprets one.
func azureCondition(condition string) string {
	condition = strings.TrimSpace(condition)
	if strings.EqualFold(condition, "succeeded()") {
		return ""
	}
	return condition
}

func joinConditions(outer, inner string) string {
	inner = azureCondition(inner)
	switch {
	case outer == "":
		return inner
	case inner == "":
		return outer
	}
	return outer + " && " + inner
}

func NewAzureParser() Parser {
	return &AzureParser{}
}
