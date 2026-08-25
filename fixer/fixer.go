package fixer

import (
	"embed"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/MustacheCase/zanadir/suggester"
	"gopkg.in/yaml.v3"
)

//go:embed templates.yaml
var templatesFS embed.FS

const (
	PlatformGitHub   = "github"
	PlatformGitLab   = "gitlab"
	PlatformCircleCI = "circleci"
)

var knownPlatforms = map[string]bool{
	PlatformGitHub:   true,
	PlatformGitLab:   true,
	PlatformCircleCI: true,
}

type Template struct {
	Tool     string `yaml:"tool"`
	Platform string `yaml:"platform"`
	Step     string `yaml:"step"`
}

type templateFile struct {
	Templates []Template `yaml:"templates"`
}

// Snippet is one ready-to-paste block for an uncovered category.
type Snippet struct {
	Category   string
	Tool       string
	Repository string
	Step       string
}

type Fixer interface {
	Snippets(suggestions []*suggester.CategorySuggestion, platform string) []Snippet
}

type service struct {
	byToolPlatform map[string]Template
}

func key(tool, platform string) string {
	return strings.ToLower(tool) + "\x00" + strings.ToLower(platform)
}

// Snippets returns one snippet per uncovered category that has a template for
// this platform. A category whose tools are all untemplated is skipped rather
// than guessed at.
func (s *service) Snippets(suggestions []*suggester.CategorySuggestion, platform string) []Snippet {
	snippets := make([]Snippet, 0, len(suggestions))
	for _, category := range suggestions {
		for _, tool := range category.Suggestions {
			tmpl, ok := s.byToolPlatform[key(tool.Name, platform)]
			if !ok {
				continue
			}
			snippets = append(snippets, Snippet{
				Category:   category.Name,
				Tool:       tool.Name,
				Repository: tool.Repository,
				Step:       strings.TrimRight(tmpl.Step, "\n"),
			})
			break
		}
	}
	return snippets
}

func readTemplates() ([]Template, error) {
	data, err := templatesFS.ReadFile("templates.yaml")
	if err != nil {
		return nil, fmt.Errorf("failed to read templates: %w", err)
	}
	var file templateFile
	if err := yaml.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("failed to parse templates: %w", err)
	}
	return file.Templates, nil
}

// validate rejects a template whose tool is not in the catalogue. The same
// silent key mismatch has bitten this project in category ids, applyOn
// selectors and Job.Name, so it fails loudly here instead.
func validate(templates []Template, catalogue []suggester.CategorySuggestion) error {
	known := make(map[string]bool)
	for _, category := range catalogue {
		for _, tool := range category.Suggestions {
			known[strings.ToLower(tool.Name)] = true
		}
	}

	var unknown []string
	for _, t := range templates {
		if t.Tool == "" || t.Step == "" {
			return fmt.Errorf("template for %q is missing a tool name or step", t.Tool)
		}
		if !knownPlatforms[t.Platform] {
			return fmt.Errorf("template for %q has unknown platform %q", t.Tool, t.Platform)
		}
		if !known[strings.ToLower(t.Tool)] {
			unknown = append(unknown, t.Tool)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return fmt.Errorf("templates reference tools absent from suggestions.yaml: %s",
			strings.Join(unknown, ", "))
	}
	return nil
}

func NewFixService() (Fixer, error) {
	templates, err := readTemplates()
	if err != nil {
		return nil, err
	}
	catalogue, err := suggester.Catalogue()
	if err != nil {
		return nil, err
	}
	if err := validate(templates, catalogue); err != nil {
		return nil, err
	}

	byToolPlatform := make(map[string]Template, len(templates))
	for _, t := range templates {
		byToolPlatform[key(t.Tool, t.Platform)] = t
	}
	return &service{byToolPlatform: byToolPlatform}, nil
}

// DetectPlatform reports which CI platform a repository uses, defaulting to
// GitHub Actions when nothing is recognised.
func DetectPlatform(dir string) string {
	if info, err := os.Stat(filepath.Join(dir, ".github", "workflows")); err == nil && info.IsDir() {
		return PlatformGitHub
	}
	if info, err := os.Stat(filepath.Join(dir, ".gitlab-ci.yml")); err == nil && !info.IsDir() {
		return PlatformGitLab
	}
	if info, err := os.Stat(filepath.Join(dir, ".circleci")); err == nil && info.IsDir() {
		return PlatformCircleCI
	}
	return PlatformGitHub
}

// targetFile names where a snippet is pasted, per platform.
func targetFile(platform string) string {
	switch platform {
	case PlatformGitLab:
		return ".gitlab-ci.yml"
	case PlatformCircleCI:
		return ".circleci/config.yml"
	default:
		return ".github/workflows/security.yml"
	}
}

// Render writes the snippets as copy-paste blocks.
func Render(w io.Writer, snippets []Snippet, platform string) error {
	if len(snippets) == 0 {
		_, err := fmt.Fprintln(w, "Nothing to fix: every uncovered category either has no template yet, or there is nothing uncovered.")
		return err
	}

	target := targetFile(platform)
	for i, s := range snippets {
		if i > 0 {
			if _, err := fmt.Fprintln(w); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintf(w, "%s is not covered. Add to %s:\n\n", s.Category, target); err != nil {
			return err
		}
		for _, line := range strings.Split(s.Step, "\n") {
			if _, err := fmt.Fprintf(w, "  %s\n", line); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintf(w, "\n  %s\n", s.Repository); err != nil {
			return err
		}
	}
	return nil
}
