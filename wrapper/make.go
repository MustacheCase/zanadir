package wrapper

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

var makefileNames = []string{"GNUmakefile", "makefile", "Makefile"}

type makeRule struct {
	prereqs []string
	recipe  []string
}

// makefile is what zanadir reads of one: variables, explicit rules and the
// match-anything rule ("%:" or ".DEFAULT:") that catches every other target.
// Conditionals are not evaluated, so both branches count.
type makefile struct {
	path     string
	parsed   map[string]bool
	vars     map[string]string
	rules    map[string]*makeRule
	anything *makeRule
	first    string
}

func (e *Expander) make(dir string, args []string) {
	file, ignore := "", false
	var goals []string
	for i := 0; i < len(args); i++ {
		switch arg := args[i]; {
		case (arg == "-C" || arg == "--directory") && i+1 < len(args):
			i++
			if next, ok := e.within(dir, args[i]); ok {
				dir = next
			}
		case (arg == "-f" || arg == "--file") && i+1 < len(args):
			i++
			file = args[i]
		case arg == "--ignore-errors" || strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "--") && strings.ContainsRune(arg, 'i'):
			ignore = true
		case strings.HasPrefix(arg, "-") || strings.Contains(arg, "="):
		default:
			goals = append(goals, arg)
		}
	}
	if file == "" {
		file = findFile(dir, makefileNames)
	}
	path, ok := e.within(dir, file)
	if file == "" || !ok {
		return
	}

	m := e.makefile(path)
	if len(goals) == 0 {
		goals = []string{m.defaultGoal()}
	}
	e.ignored(ignore, func() {
		for _, goal := range goals {
			e.makeTarget(m, dir, goal)
		}
	})
}

func (e *Expander) makefile(path string) *makefile {
	if m, ok := e.makefiles[path]; ok {
		return m
	}
	m := &makefile{path: path, parsed: map[string]bool{}, vars: map[string]string{}, rules: map[string]*makeRule{}}
	m.parse(e, path)
	e.makefiles[path] = m
	return m
}

func (m *makefile) defaultGoal() string {
	if goal := m.expand(m.vars[".DEFAULT_GOAL"]); goal != "" {
		return goal
	}
	return m.first
}

func (e *Expander) makeTarget(m *makefile, dir, target string) {
	if !e.once("make " + m.path + " " + target) {
		return
	}
	rule := m.rules[target]
	if rule == nil || len(rule.recipe) == 0 && m.anything != nil && len(rule.prereqs) == 0 {
		rule = m.anything
	}
	if rule == nil {
		return
	}
	for _, prereq := range rule.prereqs {
		e.makeTarget(m, dir, prereq)
	}
	automatic := strings.NewReplacer("$@", target, "$*", target, "$(MAKE)", "make", "${MAKE}", "make")
	ignoring := m.rules[".IGNORE"]
	ignoreAll := ignoring != nil && (len(ignoring.prereqs) == 0 || slices.Contains(ignoring.prereqs, target))
	for _, line := range rule.recipe {
		command := strings.TrimLeft(line, "@-+ \t")
		// A recipe line starting with - has its failure ignored.
		e.ignored(ignoreAll || strings.Contains(line[:len(line)-len(command)], "-"), func() {
			e.emit(dir, m.expand(automatic.Replace(command)))
		})
	}
}

var (
	makeAssignment = regexp.MustCompile(`^(?:export\s+|override\s+)?([A-Za-z0-9_.-]+)\s*(:::=|::=|:=|\?=|\+=|!=|=)\s*(.*)$`)
	makeReference  = regexp.MustCompile(`\$[({]([^(){}$]*)[)}]`)
)

// parse reads a makefile and the files it includes, each once, adding to m.
func (m *makefile) parse(e *Expander, path string) {
	data, err := os.ReadFile(path)
	if err != nil || m.parsed[path] {
		return
	}
	m.parsed[path] = true
	var current []*makeRule
	inDefine := false
	for _, line := range strings.Split(strings.ReplaceAll(string(data), "\\\n", " "), "\n") {
		if strings.HasPrefix(line, "\t") && !inDefine {
			for _, rule := range current {
				rule.recipe = append(rule.recipe, line[1:])
			}
			continue
		}
		line, _, _ = strings.Cut(line, "#")
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		switch fields[0] {
		case "endef":
			inDefine = false
			continue
		case "define":
			inDefine = true
			continue
		case "ifeq", "ifneq", "ifdef", "ifndef", "else", "endif":
			continue
		case "include", "-include", "sinclude":
			for _, pattern := range fields[1:] {
				matches, _ := filepath.Glob(filepath.Join(filepath.Dir(path), m.expand(pattern)))
				for _, match := range matches {
					if _, ok := e.within(e.root, match); ok {
						m.parse(e, match)
					}
				}
			}
			current = nil
			continue
		}
		if inDefine {
			continue
		}
		if a := makeAssignment.FindStringSubmatch(strings.TrimSpace(line)); a != nil {
			m.assign(a[1], a[2], a[3])
			current = nil
			continue
		}
		current = m.rule(line)
	}
}

func (m *makefile) assign(name, op, value string) {
	switch op {
	case "?=":
		if _, set := m.vars[name]; !set {
			m.vars[name] = value
		}
	case "+=":
		m.vars[name] = strings.TrimSpace(m.vars[name] + " " + value)
	case ":=", "::=", ":::=":
		m.vars[name] = m.expand(value)
	case "!=":
		m.vars[name] = ""
	default:
		m.vars[name] = value
	}
}

// rule reads "targets: prerequisites ; recipe" and returns the rules that
// the recipe lines after it belong to.
func (m *makefile) rule(line string) []*makeRule {
	head, inline, _ := strings.Cut(line, ";")
	targets, prereqs, ok := strings.Cut(m.expand(head), ":")
	if !ok || strings.Contains(prereqs, "=") {
		return nil
	}
	prereqs = strings.ReplaceAll(strings.TrimPrefix(prereqs, ":"), "|", " ")

	var rules []*makeRule
	for _, target := range strings.Fields(targets) {
		var rule *makeRule
		switch {
		case target == "%" || target == ".DEFAULT":
			if m.anything == nil {
				m.anything = &makeRule{}
			}
			rule = m.anything
		case strings.Contains(target, "%"):
			rule = &makeRule{}
		default:
			if rule = m.rules[target]; rule == nil {
				rule = &makeRule{}
				m.rules[target] = rule
			}
			if m.first == "" && !strings.HasPrefix(target, ".") {
				m.first = target
			}
		}
		rule.prereqs = append(rule.prereqs, strings.Fields(prereqs)...)
		if strings.TrimSpace(inline) != "" {
			rule.recipe = append(rule.recipe, inline)
		}
		rules = append(rules, rule)
	}
	return rules
}

// expand substitutes variable references. A function call such as
// $(shell ...) cannot be evaluated and expands to nothing.
func (m *makefile) expand(s string) string {
	s = strings.ReplaceAll(s, "$$", "\x00")
	for range maxDepth {
		next := makeReference.ReplaceAllStringFunc(s, func(ref string) string {
			return m.vars[ref[2:len(ref)-1]]
		})
		if next == s {
			break
		}
		s = next
	}
	return strings.ReplaceAll(s, "\x00", "$")
}
