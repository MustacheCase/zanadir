// Package fixer renders CI configuration snippets for the categories a scan reports as uncovered.
package fixer

import (
	"embed"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/MustacheCase/zanadir/internal/repofs"
	"github.com/MustacheCase/zanadir/models"
	"github.com/MustacheCase/zanadir/parser"
	"github.com/MustacheCase/zanadir/suggester"
	"gopkg.in/yaml.v3"
)

//go:embed templates.yaml
var templatesFS embed.FS

// Template is one tool's CI configuration snippet for one platform.
type Template struct {
	Tool     string `yaml:"tool"`
	Platform string `yaml:"platform"`
	Step     string `yaml:"step"`
	// Category scopes a template to one category. A tool that covers several
	// categories needs a different step for each; without this, the first
	// template for the tool wins and the generated fix does not do what the
	// category asks for.
	Category string `yaml:"category"`
	// Requires lists files at the repository root, any one of which the step
	// needs; Missing says why it cannot run without them.
	Requires []string `yaml:"requires"`
	Missing  string   `yaml:"missing"`
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

// Gap is an uncovered category with no snippet that fits the repository.
type Gap struct {
	Category string
	Reason   string
}

// Plan is a snippet for each uncovered category fix can fill, and the reason
// for each it cannot.
type Plan struct {
	Snippets []Snippet
	Gaps     []Gap
}

// Fixer turns uncovered categories into CI configuration snippets for a platform.
type Fixer interface {
	Plan(suggestions []*suggester.CategorySuggestion, platform, dir string) Plan
}

type service struct {
	byToolPlatform map[string]Template
}

func key(tool, platform string) string {
	return strings.ToLower(tool) + "\x00" + strings.ToLower(platform)
}

func categoryKey(tool, platform, category string) string {
	return key(tool, platform) + "\x00" + strings.ToLower(category)
}

// Plan picks, for each uncovered category, the first suggested tool with a
// template for this platform whose required files dir has. A category with
// none is reported as a gap rather than guessed at.
func (s *service) Plan(suggestions []*suggester.CategorySuggestion, platform, dir string) Plan {
	var plan Plan
	for _, category := range suggestions {
		if snippet, gap := s.pick(category, platform, dir); snippet != nil {
			plan.Snippets = append(plan.Snippets, *snippet)
		} else {
			plan.Gaps = append(plan.Gaps, gap)
		}
	}
	return plan
}

func (s *service) pick(category *suggester.CategorySuggestion, platform, dir string) (*Snippet, Gap) {
	var unmet string
	names := make([]string, 0, len(category.Suggestions))
	for _, tool := range category.Suggestions {
		names = append(names, tool.Name)
		tmpl, ok := s.byToolPlatform[categoryKey(tool.Name, platform, category.ID)]
		if !ok {
			tmpl, ok = s.byToolPlatform[key(tool.Name, platform)]
		}
		if !ok {
			continue
		}
		if len(tmpl.Requires) > 0 && !slices.ContainsFunc(tmpl.Requires, func(f string) bool { return repofs.Exists(dir, filepath.Join(dir, f)) }) {
			if unmet == "" {
				unmet = fmt.Sprintf("%s. If it does not apply here, exclude it with -e %q.", tmpl.Missing, category.ID)
			}
			continue
		}
		return &Snippet{
			Category:   category.Name,
			Tool:       tool.Name,
			Repository: tool.Repository,
			Step:       strings.TrimRight(tmpl.Step, "\n"),
		}, Gap{}
	}
	if unmet != "" {
		return nil, Gap{Category: category.Name, Reason: unmet}
	}
	reason := "no ready-made snippet for this repository"
	if len(names) > 0 {
		reason += "; suggested tools: " + strings.Join(names, ", ")
	}
	return nil, Gap{Category: category.Name, Reason: reason + "."}
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
		if _, ok := parser.Lookup(t.Platform); !ok {
			return fmt.Errorf("template for %q has unknown platform %q", t.Tool, t.Platform)
		}
		if t.Category != "" {
			if _, ok := models.ResolveCategory(t.Category); !ok {
				return fmt.Errorf("template for %q has unknown category %q", t.Tool, t.Category)
			}
		}
		if len(t.Requires) > 0 && t.Missing == "" {
			return fmt.Errorf("template for %q requires files but does not say why", t.Tool)
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

// NewFixService returns a Fixer backed by the embedded templates.
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
		if t.Category != "" {
			byToolPlatform[categoryKey(t.Tool, t.Platform, t.Category)] = t
			continue
		}
		byToolPlatform[key(t.Tool, t.Platform)] = t
	}
	return &service{byToolPlatform: byToolPlatform}, nil
}

// Render writes the snippets as copy-paste blocks, then the gaps.
func Render(w io.Writer, plan Plan, platform parser.Platform) error {
	if len(plan.Snippets) == 0 && len(plan.Gaps) == 0 {
		_, err := fmt.Fprintln(w, "Nothing to fix: every category is covered.")
		return err
	}

	for i, s := range plan.Snippets {
		if i > 0 {
			if _, err := fmt.Fprintln(w); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintf(w, "%s is not covered. Add to %s:\n\n", s.Category, platform.Target); err != nil {
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
	if len(plan.Snippets) > 0 && len(plan.Gaps) > 0 {
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
	}
	return RenderGaps(w, plan.Gaps)
}

// RenderGaps writes the uncovered categories fix has no snippet for.
func RenderGaps(w io.Writer, gaps []Gap) error {
	if len(gaps) == 0 {
		return nil
	}
	if _, err := fmt.Fprintln(w, "Not covered, with no snippet that fits:"); err != nil {
		return err
	}
	for _, g := range gaps {
		if _, err := fmt.Fprintf(w, "  %s: %s\n", g.Category, g.Reason); err != nil {
			return err
		}
	}
	return nil
}

const (
	generatedMarker   = "# Generated by zanadir fix --write"
	generatedWorkflow = ".github/workflows/zanadir-suggested.yml"
)

// Workflow renders the snippets as a standalone GitHub Actions workflow.
func Workflow(plan Plan) string {
	var b strings.Builder
	b.WriteString(generatedMarker + "\n")
	b.WriteString("# Regenerate with the same command; this file is overwritten.\n")
	b.WriteString("#\n")
	b.WriteString("# This is a standalone workflow, so it repeats checkout and runs as its own\n")
	b.WriteString("# job: slower and noisier in the checks list than adding these steps to a\n")
	b.WriteString("# job you already have. That is the price of not editing your workflows.\n")
	if len(plan.Gaps) > 0 {
		b.WriteString("#\n# Not included:\n")
		for _, g := range plan.Gaps {
			fmt.Fprintf(&b, "# - %s: %s\n", g.Category, g.Reason)
		}
	}
	b.WriteString("\nname: zanadir-suggested\n\non:\n  push:\n    branches: [main]\n  pull_request:\n")
	b.WriteString("\npermissions:\n  contents: read\n")
	b.WriteString("\njobs:\n  suggested:\n    runs-on: ubuntu-latest\n    steps:\n")
	// Without persist-credentials: false, zizmor reports the checkout as
	// leaving the token on disk.
	b.WriteString("      - uses: actions/checkout@v7\n        with:\n          persist-credentials: false\n")

	for _, s := range plan.Snippets {
		fmt.Fprintf(&b, "\n      # %s\n", s.Category)
		for _, line := range strings.Split(strings.TrimRight(s.Step, "\n"), "\n") {
			if line == "" {
				b.WriteString("\n")
				continue
			}
			fmt.Fprintf(&b, "      %s\n", line)
		}
	}
	return b.String()
}

// WriteWorkflow writes the standalone workflow, refusing to overwrite a file
// zanadir did not generate.
func WriteWorkflow(dir string, plan Plan) (string, error) {
	path := filepath.Join(dir, filepath.FromSlash(generatedWorkflow))

	existing, err := repofs.ReadFile(dir, path)
	switch {
	case err == nil && !strings.HasPrefix(string(existing), generatedMarker):
		return "", fmt.Errorf("refusing to overwrite %s: it was not generated by zanadir", generatedWorkflow)
	case err != nil && !errors.Is(err, fs.ErrNotExist):
		return "", fmt.Errorf("failed to read %s: %w", generatedWorkflow, err)
	}

	if err := repofs.WriteFile(dir, path, []byte(Workflow(plan)), 0o644); err != nil {
		return "", fmt.Errorf("failed to write %s: %w", generatedWorkflow, err)
	}
	return generatedWorkflow, nil
}

// IsGeneratedWorkflow reports whether a path is the workflow fix --write
// produces. Counting it as coverage would make each regeneration delete the
// steps the previous one added.
func IsGeneratedWorkflow(location string) bool {
	return filepath.Base(location) == filepath.Base(generatedWorkflow)
}
