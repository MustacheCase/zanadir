package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/MustacheCase/zanadir/suggester"
	"gopkg.in/yaml.v3"
)

// staleAfter is how long without a release a recommended tool may go.
const staleAfter = 365 * 24 * time.Hour

type finding struct {
	// Broken means zanadir recommends or pins something that is gone;
	// otherwise the finding is out of date.
	Broken bool
	Source string
	Tool   string
	Repo   string
	Detail string
}

var (
	githubRepository = regexp.MustCompile(`^https://github\.com/([A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+)/?$`)
	pinnedAction     = regexp.MustCompile(`uses:\s*([A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+)(?:/[^@\s]*)?@([^\s\\"]+)(?:[ \t]+#[ \t]*([^\s\\"]+))?`)
	commitPin        = regexp.MustCompile(`^[0-9a-f]{40}$`)
	semver           = regexp.MustCompile(`^v?\d+(?:\.\d+)*$`)
)

func check(root, token string, now time.Time) ([]finding, error) {
	gh := &github{token: token, repos: map[string]*repository{}}
	var findings []finding
	seen := map[string]bool{}
	add := func(f finding) {
		key := f.Source + f.Repo + f.Detail
		if !seen[key] {
			seen[key] = true
			findings = append(findings, f)
		}
	}

	var catalogue suggester.CategoryFile
	if err := readYAML(filepath.Join(root, "suggester", "suggestions.yaml"), &catalogue); err != nil {
		return nil, err
	}
	for _, category := range catalogue.Categories {
		for _, s := range category.Suggestions {
			match := githubRepository.FindStringSubmatch(s.Repository)
			if match == nil {
				continue
			}
			repo, err := gh.repository(match[1])
			if err != nil {
				return nil, err
			}
			for _, f := range repo.health(now) {
				f.Source, f.Tool = "suggestion", s.Name
				add(f)
			}
		}
	}

	var templates struct {
		Templates []struct {
			Tool string `yaml:"tool"`
			Step string `yaml:"step"`
		} `yaml:"templates"`
	}
	if err := readYAML(filepath.Join(root, "fixer", "templates.yaml"), &templates); err != nil {
		return nil, err
	}
	for _, t := range templates.Templates {
		for _, pin := range pinnedAction.FindAllStringSubmatch(t.Step, -1) {
			pins, err := gh.pin(pin[1], pin[2], pin[3], now)
			if err != nil {
				return nil, err
			}
			for _, f := range pins {
				f.Source, f.Tool = "fix template", t.Tool
				add(f)
			}
		}
	}

	// The workflow skeleton every fix is written into pins actions too.
	skeleton, err := os.ReadFile(filepath.Join(root, "fixer", "fixer.go"))
	if err != nil {
		return nil, err
	}
	for _, pin := range pinnedAction.FindAllStringSubmatch(string(skeleton), -1) {
		pins, err := gh.pin(pin[1], pin[2], pin[3], now)
		if err != nil {
			return nil, err
		}
		for _, f := range pins {
			f.Source, f.Tool = "fix workflow", pin[1]
			add(f)
		}
	}
	return findings, nil
}

// health judges a recommended or pinned repository: gone, archived, or
// without a release - or, having never released, a push - in a year.
func (r *repository) health(now time.Time) []finding {
	switch {
	case r.Missing:
		return []finding{{Broken: true, Repo: r.Name, Detail: "the repository does not exist"}}
	case r.Archived:
		return []finding{{Broken: true, Repo: r.Name, Detail: "the repository is archived"}}
	}
	last, what := r.Released, "release"
	if last.IsZero() {
		last, what = r.Pushed, "release or push"
	}
	if now.Sub(last) > staleAfter {
		return []finding{{Repo: r.Name, Detail: fmt.Sprintf("no %s since %s", what, last.Format("2006-01-02"))}}
	}
	return nil
}

// pin judges a template's action pin: the repository, the ref being a tag
// that exists - for a commit, the tag in its comment, at that commit - and a
// newer major being out.
func (gh *github) pin(name, ref, comment string, now time.Time) ([]finding, error) {
	repo, err := gh.repository(name)
	if err != nil {
		return nil, err
	}
	findings := repo.health(now)
	if repo.Missing || repo.Archived {
		return findings, nil
	}
	if commitPin.MatchString(ref) {
		if !repo.Tags[comment] {
			return append(findings, finding{Broken: true, Repo: name, Detail: fmt.Sprintf("@%s is not labelled with a tag", ref[:7])}), nil
		}
		at, err := gh.commitOf(name, comment)
		if err != nil {
			return nil, err
		}
		if at != ref {
			return append(findings, finding{Broken: true, Repo: name, Detail: fmt.Sprintf("@%s is not %s", ref[:7], comment)}), nil
		}
		ref = comment
	}
	if !repo.Tags[ref] {
		return append(findings, finding{Broken: true, Repo: name, Detail: fmt.Sprintf("@%s is not a tag", ref)}), nil
	}
	if pinned := versionOf(ref); pinned != nil && repo.Newest[0] > pinned[0] {
		findings = append(findings, finding{Repo: name, Detail: fmt.Sprintf("pinned to %s; v%d is out", ref, repo.Newest[0])})
	}
	return findings, nil
}

// versionOf parses a release tag such as v1.2.3, or nil for anything else,
// pre-releases included.
func versionOf(tag string) []int {
	if !semver.MatchString(tag) {
		return nil
	}
	var version []int
	for _, part := range strings.Split(strings.TrimPrefix(tag, "v"), ".") {
		n, _ := strconv.Atoi(part)
		version = append(version, n)
	}
	return version
}

func readYAML(path string, out any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := yaml.Unmarshal(data, out); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

// render writes the findings as the body of the issue the workflow keeps.
func render(findings []finding) string {
	if len(findings) == 0 {
		return "The catalogue is healthy: every recommended tool and pinned action is alive and current.\n"
	}
	var b strings.Builder
	b.WriteString("zanadir's catalogue has problems. Recommending or pinning a dead tool erodes trust in every report, so each row below needs a decision: update it, replace it, or remove it.\n")
	for _, section := range []struct {
		broken bool
		title  string
	}{{true, "Broken"}, {false, "Out of date"}} {
		var rows []string
		for _, f := range findings {
			if f.Broken == section.broken {
				rows = append(rows, fmt.Sprintf("| %s | %s | %s | %s |", f.Source, f.Tool, f.Repo, f.Detail))
			}
		}
		if len(rows) > 0 {
			fmt.Fprintf(&b, "\n## %s\n\n| Where | Tool | Repository | Problem |\n| --- | --- | --- | --- |\n%s\n", section.title, strings.Join(rows, "\n"))
		}
	}
	b.WriteString("\nOpened by the weekly catalogue health check (`tools/cataloguehealth`); it updates this issue, and closes it once the catalogue is healthy.\n")
	return b.String()
}
