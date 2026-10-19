package wrapper

import (
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"github.com/MustacheCase/zanadir/internal/repofs"
	"gopkg.in/yaml.v3"
)

var taskfileNames = []string{
	"Taskfile.yml", "taskfile.yml", "Taskfile.yaml", "taskfile.yaml",
	"Taskfile.dist.yml", "taskfile.dist.yml", "Taskfile.dist.yaml", "taskfile.dist.yaml",
}

type taskfile struct {
	path     string
	tasks    map[string]yaml.Node
	includes map[string]taskInclude
}

// taskInclude is one entry of includes:, as a path or a mapping.
type taskInclude struct {
	Taskfile string `yaml:"taskfile"`
	Dir      string `yaml:"dir"`
	Flatten  bool   `yaml:"flatten"`
}

func (i *taskInclude) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		i.Taskfile = node.Value
		return nil
	}
	type plain taskInclude
	return node.Decode((*plain)(i))
}

func (e *Expander) task(dir string, args []string) {
	file := ""
	var names []string
	for i := 0; i < len(args); i++ {
		switch arg := args[i]; {
		case arg == "--":
			i = len(args)
		case (arg == "-d" || arg == "--dir") && i+1 < len(args):
			i++
			if next, ok := e.within(dir, args[i]); ok {
				dir = next
			}
		case (arg == "-t" || arg == "--taskfile") && i+1 < len(args):
			i++
			file = args[i]
		case strings.HasPrefix(arg, "-") || strings.Contains(arg, "="):
		default:
			names = append(names, arg)
		}
	}
	if file == "" {
		file = findFile(dir, taskfileNames)
	}
	path, ok := e.within(dir, file)
	if file == "" || !ok {
		return
	}

	t := e.taskfile(path)
	if len(names) == 0 {
		names = []string{"default"}
	}
	for _, name := range names {
		e.runTask(t, filepath.Dir(path), name)
	}
}

func (e *Expander) taskfile(path string) *taskfile {
	if t, ok := e.taskfiles[path]; ok {
		return t
	}
	t := &taskfile{path: path}
	var doc struct {
		Tasks    map[string]yaml.Node   `yaml:"tasks"`
		Includes map[string]taskInclude `yaml:"includes"`
	}
	if data, err := repofs.ReadFile(e.root, path); err == nil && yaml.Unmarshal(data, &doc) == nil {
		t.tasks, t.includes = doc.Tasks, doc.Includes
	}
	e.taskfiles[path] = t
	return t
}

// included resolves a task name through includes: "ns:task" in the file the
// namespace names, nested as "a:b:task", or a task of a flattened include.
// Its tasks run in the including directory unless the include sets dir:.
func (e *Expander) included(t *taskfile, dir, name string) (*taskfile, string, string, bool) {
	for _, namespace := range slices.Sorted(maps.Keys(t.includes)) {
		include := t.includes[namespace]
		rest, found := strings.CutPrefix(name, namespace+":")
		if !found && !include.Flatten {
			continue
		}
		if !found {
			rest = name
		}
		file, ok := e.within(filepath.Dir(t.path), include.Taskfile)
		if !ok {
			continue
		}
		if names := findFile(file, taskfileNames); names != "" {
			file = filepath.Join(file, names)
		}
		if include.Dir != "" {
			if next, ok := e.within(filepath.Dir(t.path), include.Dir); ok {
				dir = next
			}
		}
		// A namespace is followed even into a nested include; a flattened
		// include answers only for the tasks it has.
		inner := e.taskfile(file)
		if _, ok := inner.tasks[rest]; ok || found {
			return inner, dir, rest, true
		}
	}
	return nil, "", "", false
}

// runTask follows a task given as one command, a list of them, or a mapping
// with deps, which run first, and cmd or cmds.
func (e *Expander) runTask(t *taskfile, dir, name string) {
	node, ok := t.tasks[name]
	if !ok {
		if inner, innerDir, rest, found := e.included(t, dir, name); found && e.depth < maxDepth {
			e.depth++
			defer func() { e.depth-- }()
			e.runTask(inner, innerDir, rest)
		}
		return
	}
	if !e.once("task " + t.path + " " + name) {
		return
	}
	switch node.Kind {
	case yaml.ScalarNode:
		e.emit(dir, node.Value)
	case yaml.SequenceNode:
		e.taskItems(t, dir, node.Content, false)
	case yaml.MappingNode:
		var def struct {
			Deps        yaml.Node `yaml:"deps"`
			Cmd         string    `yaml:"cmd"`
			Cmds        yaml.Node `yaml:"cmds"`
			IgnoreError bool      `yaml:"ignore_error"`
		}
		if node.Decode(&def) != nil {
			return
		}
		e.taskItems(t, dir, def.Deps.Content, true)
		e.ignored(def.IgnoreError, func() {
			e.emit(dir, def.Cmd)
			e.taskItems(t, dir, def.Cmds.Content, false)
		})
	}
}

// taskItems follows deps or cmds. Either may name another task in a mapping;
// a plain string is a task name among deps and a command among cmds.
func (e *Expander) taskItems(t *taskfile, dir string, items []*yaml.Node, plainIsTask bool) {
	for _, item := range items {
		var ref struct {
			Task, Cmd   string
			IgnoreError bool `yaml:"ignore_error"`
		}
		switch {
		case item.Kind == yaml.ScalarNode && plainIsTask:
			e.runTask(t, dir, item.Value)
		case item.Kind == yaml.ScalarNode:
			e.emit(dir, item.Value)
		case item.Decode(&ref) != nil:
		case ref.Task != "":
			e.runTask(t, dir, ref.Task)
		default:
			e.ignored(ref.IgnoreError, func() { e.emit(dir, ref.Cmd) })
		}
	}
}
