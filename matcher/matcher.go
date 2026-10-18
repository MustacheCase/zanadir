package matcher

import (
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/MustacheCase/zanadir/internal/shell"
	"github.com/MustacheCase/zanadir/models"
	"github.com/MustacheCase/zanadir/rules"
)

type Finding struct {
	Category string
	RuleID   string
	Location string
	// Job is nil when the match was on the workflow name.
	Artifact *models.Artifact
	Job      *models.Job
	Rule     *rules.Rule
}

type Matcher interface {
	Match([]*models.Artifact, []*rules.Rule) []*Finding
}

type service struct{}

func (s *service) Match(artifacts []*models.Artifact, ruleSet []*rules.Rule) []*Finding {
	var findings []*Finding

	for _, rule := range ruleSet {
		for _, artifact := range artifacts {
			for _, job := range matchingJobs(artifact, rule) {
				for _, c := range rule.Categories {
					findings = append(findings, &Finding{
						Category: c,
						RuleID:   rule.ID,
						Location: artifact.Location,
						Artifact: artifact,
						Job:      job,
						Rule:     rule,
					})
				}
			}
		}
	}

	return findings
}

// matchingJobs returns every step a rule matches. A match on the workflow's
// name belongs to no step and yields a single nil, but only when no step
// matches: a step says how the tool is wired, which grading needs, while a
// name match would outrank it ungraded.
func matchingJobs(artifact *models.Artifact, rule *rules.Rule) []*models.Job {
	var matched []*models.Job
	named := false
	for _, field := range rule.ApplyOn {
		if field == rules.FieldArtifactName {
			named = named || rule.Regex.MatchString(artifact.Name)
			continue
		}
		matched = append(matched, matchingSteps(artifact, rule, field)...)
	}
	if named && len(matched) == 0 {
		return []*models.Job{nil}
	}
	return matched
}

func matchingSteps(artifact *models.Artifact, rule *rules.Rule, field string) []*models.Job {
	var matched []*models.Job
	for _, job := range artifact.Jobs {
		var text string
		switch field {
		case rules.FieldJobName:
			text = job.Name
		case rules.FieldJobPackage:
			if !setupAction.MatchString(job.Package) {
				text = job.Package
			}
		case rules.FieldJobRun:
			text = invocations(job.Run)
		default:
			continue
		}
		// A rule that can match "" would otherwise match every step that
		// lacks the field.
		if text != "" && rule.Regex.MatchString(text) {
			matched = append(matched, job)
		}
	}
	return matched
}

// passiveCommands handle or fetch a tool's files without running it: making
// a freshly built grype executable, or downloading its installer, scans
// nothing.
var passiveCommands = set("cat", "chmod", "chown", "cp", "curl", "echo", "file", "install", "ln", "ls",
	"mkdir", "mv", "printf", "rm", "stat", "tar", "touch", "unzip", "wget", "which")

// A package manager installs a tool when one of its first two words says so:
// "pip install bandit", "uv tool install ruff", "go install …/gosec".
var (
	packageManagers = set("apk", "apt", "apt-get", "brew", "bun", "cargo", "choco", "composer", "conda", "dnf",
		"dotnet", "gem", "go", "mamba", "npm", "pip", "pip3", "pipx", "pnpm", "poetry", "scoop", "uv", "yarn", "yum")
	installing = set("add", "binstall", "ci", "download", "get", "i", "inject", "install", "require")
	// Asking a tool for its version or help runs no scan.
	selfDescribing = set("--help", "--version", "-V", "-h", "-version", "help", "version")
)

// setupAction names an action that installs a tool for later steps.
var setupAction = regexp.MustCompile(`(?i)(?:^|/)(?:setup|install|download)(?:-[^/]*)?$|-(?:installer|setup)$`)

func set(words ...string) map[string]bool {
	m := make(map[string]bool, len(words))
	for _, w := range words {
		m[w] = true
	}
	return m
}

// passive reports whether a command handles, installs or describes a tool
// rather than running it.
func passive(words []string) bool {
	if len(words) > 2 && strings.HasPrefix(filepath.Base(words[0]), "python") && words[1] == "-m" {
		words = words[2:]
	}
	program := filepath.Base(words[0])
	switch {
	case passiveCommands[program]:
		return true
	case len(words) == 2 && selfDescribing[words[1]]:
		return true
	case packageManagers[program]:
		return slices.ContainsFunc(words[1:min(3, len(words))], func(w string) bool { return installing[w] })
	}
	return false
}

// invocations keeps the commands of a run script that run something, one per
// line.
func invocations(run string) string {
	var kept []string
	for _, command := range shell.Commands(run) {
		if words := shell.Words(command); len(words) > 0 && !passive(words) {
			kept = append(kept, command)
		}
	}
	return strings.Join(kept, "\n")
}

func NewMatchService() Matcher {
	return &service{}
}
