package wrapper

import (
	"encoding/json"
	"maps"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/MustacheCase/zanadir/internal/repofs"
	"gopkg.in/yaml.v3"
)

const maxWorkspaces = 500

type nodePackage struct {
	Name       string            `json:"name"`
	Scripts    map[string]string `json:"scripts"`
	Workspaces json.RawMessage   `json:"workspaces"`
}

var (
	yarnCommands = set("add", "audit", "autoclean", "bin", "cache", "check", "config", "constraints", "create", "dlx", "exec", "explain",
		"generate-lock-entry", "global", "help", "import", "info", "init", "install", "licenses", "link", "list", "login", "logout", "node",
		"npm", "outdated", "owner", "pack", "patch", "plugin", "policies", "publish", "rebuild", "remove", "set", "stage", "tag", "team",
		"unlink", "unplug", "up", "upgrade", "upgrade-interactive", "version", "versions", "why")
	pnpmCommands = set("add", "approve-builds", "audit", "bin", "cat-file", "config", "create", "deploy", "dlx", "doctor", "env", "exec",
		"fetch", "i", "import", "init", "install", "licenses", "link", "list", "ls", "outdated", "pack", "patch", "prune", "publish",
		"rebuild", "remove", "rm", "root", "self-update", "server", "setup", "store", "unlink", "up", "update", "why")
	bunCommands = set("a", "add", "audit", "build", "create", "exec", "i", "info", "init", "install", "link", "outdated", "patch", "pm",
		"publish", "remove", "repl", "rm", "test", "unlink", "update", "upgrade", "why", "x")
	nxCommands = set("add", "connect", "daemon", "exec", "format", "format:check", "format:write", "g", "generate", "graph", "import",
		"init", "list", "migrate", "release", "repair", "report", "reset", "show", "sync", "view-logs", "watch")
)

func set(words ...string) map[string]bool {
	m := map[string]bool{}
	for _, w := range words {
		m[w] = true
	}
	return m
}

// nodeArgs splits a command line into its flags and positional words; the
// flags named in valued take the next word as their value.
func nodeArgs(args []string, valued ...string) (map[string]string, []string) {
	flags, positional := map[string]string{}, []string(nil)
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--":
			return flags, positional
		case strings.HasPrefix(arg, "-"):
			name, value, _ := strings.Cut(arg, "=")
			if slices.Contains(valued, name) && !strings.Contains(arg, "=") && i+1 < len(args) {
				i++
				value = args[i]
			}
			flags[name] = value
		default:
			positional = append(positional, arg)
		}
	}
	return flags, positional
}

func (e *Expander) npm(dir string, args []string) {
	flags, pos := nodeArgs(args, "--prefix", "-w", "--workspace")
	dir = e.chdir(dir, flags, "--prefix")
	if len(pos) == 0 {
		return
	}
	name := ""
	switch pos[0] {
	case "run", "run-script", "rum", "urn":
		if len(pos) > 1 {
			name = pos[1]
		}
	case "test", "t", "tst":
		name = "test"
	}
	dirs := []string{dir}
	if w := cmpFlag(flags, "-w", "--workspace"); w != "" {
		dirs = e.workspaceNamed(dir, w)
	} else if hasFlag(flags, "-ws", "--workspaces") {
		dirs = e.workspaces(dir)
	}
	e.runScripts(dirs, name, true)
}

func (e *Expander) yarn(dir string, args []string) {
	flags, pos := nodeArgs(args, "--cwd", "--exclude", "--include", "--from", "-j", "--jobs")
	dir = e.chdir(dir, flags, "--cwd")
	switch {
	case len(pos) == 0:
	case pos[0] == "workspace" && len(pos) > 2:
		e.runScripts(e.workspaceNamed(dir, pos[1]), scriptName(pos[2:]), true)
	case pos[0] == "workspaces" && len(pos) > 1:
		rest := pos[1:]
		if rest[0] == "foreach" {
			rest = rest[1:]
		}
		e.runScripts(e.workspaces(dir), scriptName(rest), true)
	case pos[0] == "run" || !yarnCommands[pos[0]]:
		e.scriptOrBinary(dir, args, scriptName(pos), true)
	}
}

func (e *Expander) pnpm(dir string, args []string) {
	flags, pos := nodeArgs(args, "-C", "--dir", "--filter", "-F", "--filter-prod", "--workspace-concurrency", "--reporter")
	dir = e.chdir(dir, flags, "-C", "--dir")
	if len(pos) == 0 || pnpmCommands[pos[0]] {
		return
	}
	name := scriptName(pos)
	if pos[0] == "test" || pos[0] == "t" || pos[0] == "tst" {
		name = "test"
	}
	if hasFlag(flags, "-r", "--recursive", "--filter", "-F", "--filter-prod") {
		e.runScripts(e.workspaces(dir), name, false)
		return
	}
	e.scriptOrBinary(dir, args, name, false)
}

func (e *Expander) bun(dir string, args []string) {
	flags, pos := nodeArgs(args, "--cwd", "--filter", "-F")
	dir = e.chdir(dir, flags, "--cwd")
	if len(pos) == 0 || bunCommands[pos[0]] {
		return
	}
	if hasFlag(flags, "--filter", "-F") {
		e.runScripts(e.workspaces(dir), scriptName(pos), false)
		return
	}
	e.scriptOrBinary(dir, args, scriptName(pos), false)
}

// turbo, lerna and nx run a package script in each workspace that has it.
func (e *Expander) turbo(dir string, args []string) {
	_, pos := nodeArgs(args, "--filter", "-F", "--concurrency", "--cache-dir", "--output-logs", "--log-order", "--env-mode", "--cache", "--ui", "--graph")
	if len(pos) > 0 && pos[0] == "run" {
		pos = pos[1:]
	}
	for _, name := range pos {
		e.runScripts(e.workspaces(dir), name, false)
	}
}

func (e *Expander) lerna(dir string, args []string) {
	_, pos := nodeArgs(args, "--scope", "--ignore", "--concurrency")
	if len(pos) > 1 && pos[0] == "run" {
		e.runScripts(e.workspaces(dir), pos[1], false)
	}
}

func (e *Expander) nx(dir string, args []string) {
	flags, pos := nodeArgs(args, "-t", "--target", "--targets", "-p", "--projects", "--exclude", "-c", "--configuration", "--base", "--head")
	switch {
	case len(pos) == 0:
	case pos[0] == "run-many" || pos[0] == "affected":
		for _, target := range strings.Split(cmpFlag(flags, "-t", "--target", "--targets"), ",") {
			e.runScripts(e.workspaces(dir), target, false)
		}
	case strings.Contains(pos[len(pos)-1], ":") && (pos[0] == "run" || len(pos) == 1):
		project, target, _ := strings.Cut(pos[len(pos)-1], ":")
		target, _, _ = strings.Cut(target, ":")
		e.runScripts(e.workspaceNamed(dir, project), target, false)
	case len(pos) > 1 && !nxCommands[pos[0]] && pos[0] != "run":
		e.runScripts(e.workspaceNamed(dir, pos[1]), pos[0], false)
	}
}

// runAll follows npm-run-all, run-s and run-p: each argument names a script,
// with its arguments after a space, or a glob over names split at ":".
func (e *Expander) runAll(dir string, args []string) {
	flags, pos := nodeArgs(args, "--npm-path", "--max-parallel")
	p := e.pkg(dir)
	if p == nil {
		return
	}
	names := slices.Sorted(maps.Keys(p.Scripts))
	e.ignored(hasFlag(flags, "-c", "--continue-on-error"), func() {
		for _, arg := range pos {
			pattern, _, _ := strings.Cut(strings.Trim(arg, `"'`), " ")
			for _, name := range names {
				if scriptMatches(pattern, name) {
					e.runScripts([]string{dir}, name, true)
				}
			}
		}
	})
}

func scriptMatches(pattern, name string) bool {
	if prefix, ok := strings.CutSuffix(pattern, "**"); ok {
		return strings.HasPrefix(name, prefix)
	}
	matched, _ := path.Match(strings.ReplaceAll(pattern, ":", "/"), strings.ReplaceAll(name, ":", "/"))
	return matched
}

// scriptOrBinary runs a script of the package in dir, else, as yarn, pnpm and
// bun do, the program of that name: "pnpm turbo run lint" runs turbo.
func (e *Expander) scriptOrBinary(dir string, args []string, name string, prePost bool) {
	if p := e.pkg(dir); p != nil && p.Scripts[name] != "" {
		e.runScripts([]string{dir}, name, prePost)
		return
	}
	if i := slices.Index(args, name); i >= 0 && name != "" {
		e.script(dir, strings.Join(args[i:], " "))
	}
}

func (e *Expander) runScripts(dirs []string, name string, prePost bool) {
	for _, dir := range dirs {
		p := e.pkg(dir)
		if name == "" || p == nil || p.Scripts[name] == "" || !e.once("script "+dir+" "+name) {
			continue
		}
		if prePost {
			e.emit(dir, p.Scripts["pre"+name])
		}
		e.emit(dir, p.Scripts[name])
		if prePost {
			e.emit(dir, p.Scripts["post"+name])
		}
	}
}

func scriptName(pos []string) string {
	switch {
	case len(pos) > 1 && (pos[0] == "run" || pos[0] == "run-script"):
		return pos[1]
	case len(pos) > 0 && pos[0] != "run":
		return pos[0]
	}
	return ""
}

func (e *Expander) chdir(dir string, flags map[string]string, names ...string) string {
	if value := cmpFlag(flags, names...); value != "" {
		if next, ok := e.within(dir, value); ok {
			return next
		}
	}
	return dir
}

func cmpFlag(flags map[string]string, names ...string) string {
	for _, name := range names {
		if flags[name] != "" {
			return flags[name]
		}
	}
	return ""
}

func hasFlag(flags map[string]string, names ...string) bool {
	return slices.ContainsFunc(names, func(name string) bool { _, ok := flags[name]; return ok })
}

func (e *Expander) pkg(dir string) *nodePackage {
	if p, ok := e.packages[dir]; ok {
		return p
	}
	var p *nodePackage
	if data, err := repofs.ReadFile(e.root, filepath.Join(dir, "package.json")); err == nil {
		var parsed nodePackage
		if json.Unmarshal(data, &parsed) == nil {
			p = &parsed
		}
	}
	e.packages[dir] = p
	return p
}

// workspaces lists the packages a package.json's workspaces or a
// pnpm-workspace.yaml names. A ** matches one directory level.
func (e *Expander) workspaces(dir string) []string {
	if dirs, ok := e.workspaceDirs[dir]; ok {
		return dirs
	}
	var patterns []string
	if p := e.pkg(dir); p != nil && len(p.Workspaces) > 0 {
		var nested struct {
			Packages []string `json:"packages"`
		}
		if json.Unmarshal(p.Workspaces, &patterns) != nil && json.Unmarshal(p.Workspaces, &nested) == nil {
			patterns = nested.Packages
		}
	}
	if data, err := repofs.ReadFile(e.root, filepath.Join(dir, "pnpm-workspace.yaml")); err == nil {
		var ws struct {
			Packages []string `yaml:"packages"`
		}
		if yaml.Unmarshal(data, &ws) == nil {
			patterns = append(patterns, ws.Packages...)
		}
	}
	var dirs, excluded []string
	for i, pattern := range patterns {
		pattern = filepath.Join(dir, filepath.FromSlash(strings.ReplaceAll(strings.TrimPrefix(strings.TrimPrefix(pattern, "!"), "./"), "**", "*")))
		if strings.HasPrefix(patterns[i], "!") {
			excluded = append(excluded, pattern)
		}
		patterns[i] = pattern
	}
	for _, pattern := range patterns {
		if slices.Contains(excluded, pattern) {
			continue
		}
		matches, _ := filepath.Glob(pattern)
		for _, match := range matches {
			if path, ok := e.within(dir, match); ok && len(dirs) < maxWorkspaces && !slices.Contains(dirs, path) &&
				!slices.ContainsFunc(excluded, func(x string) bool { ok, _ := filepath.Match(x, path); return ok }) && e.pkg(path) != nil {
				dirs = append(dirs, path)
			}
		}
	}
	e.workspaceDirs[dir] = dirs
	return dirs
}

// workspaceNamed finds a workspace by its package name or its path.
func (e *Expander) workspaceNamed(dir, name string) []string {
	for _, ws := range e.workspaces(dir) {
		if e.pkg(ws).Name == name || ws == filepath.Join(dir, name) {
			return []string{ws}
		}
	}
	return nil
}
