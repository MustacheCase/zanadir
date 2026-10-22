// Package wrapper follows the build tools a CI step calls into the commands
// they stand for, so a linter run by "make lint" is seen as a linter.
package wrapper

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/MustacheCase/zanadir/internal/shell"
)

// Bounds no real build reaches, since one wrapper may call another.
const (
	maxDepth = 8
	maxLines = 2000
)

// Expander reads the build files of one repository, each once.
type Expander struct {
	root      string
	makefiles map[string]*makefile
	taskfiles map[string]*taskfile
	toxfiles  map[string]map[string]map[string]string
	gomakes   map[string]*gomakeBuild
	packages  map[string]*nodePackage

	workspaceDirs map[string][]string

	seen  map[string]bool
	lines []string
	depth int
	// ignoring counts the wrappers, around what is being followed, that
	// discard a failure.
	ignoring int
}

func New(root string) *Expander {
	return &Expander{
		root:      root,
		makefiles: map[string]*makefile{},
		taskfiles: map[string]*taskfile{},
		toxfiles:  map[string]map[string]map[string]string{},
		gomakes:   map[string]*gomakeBuild{},
		packages:  map[string]*nodePackage{},

		workspaceDirs: map[string][]string{},
	}
}

// Expand returns the commands that a script's make, task and tox calls stand
// for, one per line.
func (e *Expander) Expand(script string) string {
	e.seen, e.lines = map[string]bool{}, nil
	e.script(e.root, script)
	return strings.Join(e.lines, "\n")
}

func (e *Expander) script(dir, script string) {
	if e.depth >= maxDepth {
		return
	}
	e.depth++
	defer func() { e.depth-- }()
	for _, command := range shell.Commands(script) {
		words := unwrap(shell.Words(command))
		if len(words) == 0 {
			continue
		}
		if filepath.Base(words[0]) == "cd" {
			if len(words) > 1 {
				if next, ok := e.within(dir, words[1]); ok {
					dir = next
				}
			}
			continue
		}
		e.ignored(shell.SwallowedCommand(script, command), func() { e.follow(dir, words) })
	}
}

// follow expands one command that runs a build tool.
func (e *Expander) follow(dir string, words []string) {
	switch filepath.Base(words[0]) {
	case "make", "gmake":
		e.make(dir, words[1:])
	case "task":
		e.task(dir, words[1:])
	case "tox":
		e.tox(dir, words[1:])
	case "go":
		e.goRun(dir, words[1:])
	case "goreleaser":
		e.goreleaser(dir, words[1:])
	case "npm":
		e.npm(dir, words[1:])
	case "yarn":
		e.yarn(dir, words[1:])
	case "pnpm":
		e.pnpm(dir, words[1:])
	case "bun":
		e.bun(dir, words[1:])
	case "turbo":
		e.turbo(dir, words[1:])
	case "lerna":
		e.lerna(dir, words[1:])
	case "nx":
		e.nx(dir, words[1:])
	case "npm-run-all", "npm-run-all2", "run-s", "run-p":
		e.runAll(dir, words[1:])
	case "pre-commit", "prek":
		if len(words) > 1 && words[1] == "run" {
			e.precommit(dir, words[2:])
		}
	}
}

// emit records a command a wrapper stands for and follows it in turn.
func (e *Expander) emit(dir, command string) {
	if command = strings.TrimSpace(command); command == "" || len(e.lines) >= maxLines {
		return
	}
	line := command
	if e.ignoring > 0 {
		line += " || true"
	}
	e.lines = append(e.lines, line)
	e.script(dir, command)
}

// ignored follows what do emits as a wrapper that discards its failure, so
// grading reads it as it reads "|| true".
func (e *Expander) ignored(ignore bool, do func()) {
	if ignore {
		e.ignoring++
		defer func() { e.ignoring-- }()
	}
	do()
}

// once reports whether key is new, so a target in a cycle is followed once.
func (e *Expander) once(key string) bool {
	if e.seen[key] {
		return false
	}
	e.seen[key] = true
	return true
}

// within resolves path against dir, refusing anything outside the repository.
func (e *Expander) within(dir, path string) (string, bool) {
	if !filepath.IsAbs(path) {
		path = filepath.Join(dir, path)
	}
	rel, err := filepath.Rel(e.root, path)
	return path, err == nil && !strings.HasPrefix(rel, "..")
}

// runPrefixes start another program: "uv run tox" runs tox.
var runPrefixes = map[string]bool{"uv": true, "poetry": true, "pdm": true, "pipx": true, "rye": true}

func unwrap(words []string) []string {
	for len(words) > 0 && (words[0] == "if" || words[0] == "elif" || words[0] == "while" || words[0] == "until" || words[0] == "!") {
		words = words[1:]
	}
	switch {
	case len(words) > 2 && runPrefixes[words[0]] && words[1] == "run":
		words = words[2:]
		for len(words) > 0 && strings.HasPrefix(words[0], "-") {
			words = words[1:]
		}
	case len(words) > 2 && strings.HasPrefix(words[0], "python") && words[1] == "-m":
		words = words[2:]
	case len(words) > 1 && (words[0] == "npx" || words[0] == "bunx" || words[0] == "corepack"):
		words = words[1:]
		for len(words) > 1 && strings.HasPrefix(words[0], "-") {
			words = words[1:]
		}
	case len(words) > 2 && (words[0] == "pnpm" || words[0] == "yarn" || words[0] == "npm") && (words[1] == "exec" || words[1] == "dlx"):
		words = words[2:]
	}
	return words
}

// findFile returns the first of names in dir, compared exactly, so a
// case-insensitive filesystem cannot give one file two names.
func findFile(dir string, names []string) string {
	entries, _ := os.ReadDir(dir)
	for _, name := range names {
		if slices.ContainsFunc(entries, func(entry os.DirEntry) bool {
			return entry.Name() == name && entry.Type().IsRegular()
		}) {
			return name
		}
	}
	return ""
}
