package wrapper

import (
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

var taskfileNames = []string{
	"Taskfile.yml", "taskfile.yml", "Taskfile.yaml", "taskfile.yaml",
	"Taskfile.dist.yml", "taskfile.dist.yml", "Taskfile.dist.yaml", "taskfile.dist.yaml",
}

type taskfile struct {
	path  string
	tasks map[string]yaml.Node
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
		Tasks map[string]yaml.Node `yaml:"tasks"`
	}
	if data, err := os.ReadFile(path); err == nil && yaml.Unmarshal(data, &doc) == nil {
		t.tasks = doc.Tasks
	}
	e.taskfiles[path] = t
	return t
}

// runTask follows a task given as one command, a list of them, or a mapping
// with deps, which run first, and cmd or cmds.
func (e *Expander) runTask(t *taskfile, dir, name string) {
	node, ok := t.tasks[name]
	if !ok || !e.once("task "+t.path+" "+name) {
		return
	}
	switch node.Kind {
	case yaml.ScalarNode:
		e.emit(dir, node.Value)
	case yaml.SequenceNode:
		e.taskItems(t, dir, node.Content, false)
	case yaml.MappingNode:
		var def struct {
			Deps yaml.Node `yaml:"deps"`
			Cmd  string    `yaml:"cmd"`
			Cmds yaml.Node `yaml:"cmds"`
		}
		if node.Decode(&def) != nil {
			return
		}
		e.taskItems(t, dir, def.Deps.Content, true)
		e.emit(dir, def.Cmd)
		e.taskItems(t, dir, def.Cmds.Content, false)
	}
}

// taskItems follows deps or cmds. Either may name another task in a mapping;
// a plain string is a task name among deps and a command among cmds.
func (e *Expander) taskItems(t *taskfile, dir string, items []*yaml.Node, plainIsTask bool) {
	for _, item := range items {
		var ref struct{ Task, Cmd string }
		switch {
		case item.Kind == yaml.ScalarNode && plainIsTask:
			e.runTask(t, dir, item.Value)
		case item.Kind == yaml.ScalarNode:
			e.emit(dir, item.Value)
		case item.Decode(&ref) != nil:
		case ref.Task != "":
			e.runTask(t, dir, ref.Task)
		default:
			e.emit(dir, ref.Cmd)
		}
	}
}
