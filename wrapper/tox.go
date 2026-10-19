package wrapper

import (
	"cmp"
	"path/filepath"
	"strings"

	"github.com/MustacheCase/zanadir/internal/repofs"
)

// tox runs each environment's commands: those of [testenv:name], or of
// [testenv] for an environment without a section of its own.
func (e *Expander) tox(dir string, args []string) {
	var envs []string
	for i := 0; i < len(args) && args[i] != "--"; i++ {
		if (args[i] == "-e" || args[i] == "--env") && i+1 < len(args) {
			i++
			envs = strings.Split(strings.Trim(args[i], `"'`), ",")
		}
	}
	path := filepath.Join(dir, "tox.ini")
	ini := e.toxfile(path)
	if len(envs) == 0 {
		envs = strings.FieldsFunc(ini["tox"]["envlist"]+","+ini["tox"]["env_list"], func(r rune) bool {
			return r == ',' || r == '\n' || r == ' '
		})
	}
	if len(envs) == 0 {
		envs = []string{""}
	}
	for _, env := range envs {
		section := "testenv:" + env
		if _, ok := ini[section]; !ok {
			section = "testenv"
		}
		if !e.once("tox " + path + " " + section) {
			continue
		}
		// ignore_outcome reports a failing environment as a warning, and a
		// command starting with - has its exit code ignored.
		outcome := strings.EqualFold(cmp.Or(ini[section]["ignore_outcome"], ini["testenv"]["ignore_outcome"]), "true")
		for _, command := range strings.Split(ini[section]["commands"], "\n") {
			command = strings.TrimSpace(command)
			e.ignored(outcome || strings.HasPrefix(command, "-"), func() {
				e.emit(dir, strings.TrimSpace(strings.TrimPrefix(command, "-")))
			})
		}
	}
}

// toxfile reads tox.ini as sections of keys, joining a value's indented
// continuation lines with newlines.
func (e *Expander) toxfile(path string) map[string]map[string]string {
	if ini, ok := e.toxfiles[path]; ok {
		return ini
	}
	ini := map[string]map[string]string{}
	e.toxfiles[path] = ini
	data, err := repofs.ReadFile(e.root, path)
	if err != nil {
		return ini
	}
	var section, key string
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, ";"):
		case strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]"):
			section, key = strings.TrimSpace(trimmed[1:len(trimmed)-1]), ""
			ini[section] = map[string]string{}
		case line[0] == ' ' || line[0] == '\t':
			if key != "" {
				ini[section][key] += "\n" + trimmed
			}
		default:
			name, value, ok := strings.Cut(trimmed, "=")
			if !ok || section == "" {
				continue
			}
			key = strings.ToLower(strings.TrimSpace(name))
			ini[section][key] = strings.TrimSpace(value)
		}
	}
	return ini
}
