// Package skills finds the agent skills in a repository and checks them for
// problems that stop them loading, triggering or being safe to run.
package skills

import (
	"bytes"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"

	"github.com/MustacheCase/zanadir/internal/repofs"
	"github.com/MustacheCase/zanadir/utils"
	"gopkg.in/yaml.v3"
)

// FileName is the file that makes a directory a skill.
const FileName = "SKILL.md"

// Skill is one SKILL.md and what its frontmatter declares.
type Skill struct {
	// Path is the SKILL.md path relative to the scanned directory, with slashes.
	Path string
	// Dir is the skill's directory relative to the scanned directory.
	Dir          string
	Name         string
	Description  string
	AllowedTools string
	// HasFrontmatter is false when the file does not open with a --- block.
	HasFrontmatter bool
	// FrontmatterErr is why the frontmatter could not be read, if it could not.
	FrontmatterErr error
	Body           string
	Content        string
	keyLines       map[string]int
}

// Discover returns every skill under dir, ordered by path. Dependency, test
// and .git directories are skipped, and no file outside dir is read.
func Discover(dir string) ([]*Skill, error) {
	var found []*Skill
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != dir && skipped(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() != FileName {
			return nil
		}
		data, err := repofs.ReadFile(dir, path)
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(dir, path)
		found = append(found, Parse(filepath.ToSlash(rel), data))
		return nil
	})
	return found, err
}

// Present reports whether dir holds a skill. It looks at names alone, so it
// also works on a fetched repository, whose other files are empty.
func Present(dir string) bool {
	found := false
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return nil
		case d.IsDir():
			if path != dir && skipped(d.Name()) {
				return filepath.SkipDir
			}
		case d.Name() == FileName:
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	return found
}

func skipped(name string) bool {
	return utils.IsDependencyDir(name) || utils.IsTestDir(name)
}

// Parse reads a SKILL.md whose path, relative to the scanned directory, is path.
func Parse(path string, data []byte) *Skill {
	text := strings.ReplaceAll(string(bytes.TrimPrefix(data, []byte("\uFEFF"))), "\r\n", "\n")
	s := &Skill{Path: path, Dir: dirOf(path), Content: text, Body: text, keyLines: map[string]int{}}

	rest, ok := strings.CutPrefix(text, "---\n")
	if !ok {
		return s
	}
	front, body, closed := strings.Cut(rest, "\n---\n")
	if !closed {
		front, closed = strings.CutSuffix(rest, "\n---")
	}
	if !closed {
		return s
	}
	s.HasFrontmatter, s.Body = true, body

	var fields map[string]any
	if err := yaml.Unmarshal([]byte(front), &fields); err != nil {
		s.FrontmatterErr = err
		return s
	}
	for i, line := range strings.Split(front, "\n") {
		if key, _, found := strings.Cut(line, ":"); found && !strings.HasPrefix(key, " ") {
			s.keyLines[key] = i + 2
		}
	}
	for _, key := range []string{"name", "description"} {
		value, present := fields[key]
		if _, isString := value.(string); present && value != nil && !isString {
			s.FrontmatterErr = fmt.Errorf("%s must be a string", key)
			return s
		}
	}
	s.Name, _ = fields["name"].(string)
	s.Description, _ = fields["description"].(string)
	s.Name, s.Description = strings.TrimSpace(s.Name), strings.TrimSpace(s.Description)
	s.AllowedTools = toolsText(fields["allowed-tools"])
	return s
}

// line is the line of a frontmatter key, or 1 when it is absent.
func (s *Skill) line(key string) int {
	if n, ok := s.keyLines[key]; ok {
		return n
	}
	return 1
}

func toolsText(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case []any:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			parts = append(parts, fmt.Sprint(item))
		}
		return strings.Join(parts, ", ")
	}
	return ""
}

func dirOf(path string) string {
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[:i]
	}
	return "."
}

// DirName is the name of the directory holding the skill.
func (s *Skill) DirName() string {
	parts := strings.Split(s.Dir, "/")
	return parts[len(parts)-1]
}

// Names lists the skills' directories, for reports.
func Names(found []*Skill) []string {
	names := make([]string, 0, len(found))
	for _, s := range found {
		names = append(names, s.Dir)
	}
	slices.Sort(names)
	return names
}

// scope is the skills directory an agent loads this skill from, such as
// .claude/skills; skills in different scopes are never loaded together.
func (s *Skill) scope() string {
	parts := strings.Split(s.Dir, "/")
	for i := len(parts) - 1; i >= 0; i-- {
		if parts[i] == "skills" {
			return strings.Join(parts[:i+1], "/")
		}
	}
	return ""
}
