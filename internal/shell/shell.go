// Package shell reads the commands of a CI step's script without running it.
package shell

import (
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

var separator = regexp.MustCompile(`\|\||&&|[;|\n]`)

// Commands splits a script into commands, dropping comments: they name tools
// without running them.
func Commands(script string) []string {
	var commands []string
	for _, command := range separator.Split(strings.ReplaceAll(script, "\\\n", " "), -1) {
		if command = strings.TrimSpace(command); command != "" && !strings.HasPrefix(command, "#") {
			commands = append(commands, command)
		}
	}
	return commands
}

// Words returns a command's program and arguments, past sudo and VAR=value.
func Words(command string) []string {
	words := strings.Fields(command)
	for len(words) > 0 && (words[0] == "sudo" || strings.Contains(words[0], "=")) {
		words = words[1:]
	}
	return words
}

// passiveCommands handle or fetch a tool's files without running it: making
// a freshly built grype executable, or downloading its installer, scans
// nothing.
var passiveCommands = set("cat", "chmod", "chown", "cp", "curl", "echo", "file", "install", "ln", "ls",
	"mkdir", "mv", "printf", "rm", "stat", "tar", "touch", "unzip", "wget", "which")

// A package manager installs a tool when one of its first two words says so:
// "pip install bandit", "uv tool install ruff", "go install …/gosec".
var (
	packageManagers = set("apk", "apt", "apt-get", "brew", "bun", "cargo", "choco", "composer", "conda", "dnf",
		"dotnet", "gem", "go", "mamba", "npm", "pip", "pip3", "pipx", "pnpm", "poetry", "scoop", "uv", "yarn", "yum")
	installing = set("add", "binstall", "ci", "download", "get", "i", "inject", "install", "require")
	// Asking a tool for its version or help runs no scan.
	selfDescribing = set("--help", "--version", "-V", "-h", "-version", "help", "version")
)

func set(words ...string) map[string]bool {
	m := make(map[string]bool, len(words))
	for _, w := range words {
		m[w] = true
	}
	return m
}

// Passive reports whether a command handles, installs or describes a tool
// rather than running it.
func Passive(words []string) bool {
	if len(words) > 2 && strings.HasPrefix(filepath.Base(words[0]), "python") && words[1] == "-m" {
		words = words[2:]
	}
	program := filepath.Base(words[0])
	switch {
	case passiveCommands[program]:
		return true
	case len(words) == 2 && selfDescribing[words[1]]:
		return true
	case packageManagers[program]:
		return slices.ContainsFunc(words[1:min(3, len(words))], func(w string) bool { return installing[w] })
	}
	return false
}

var (
	statementEnd = regexp.MustCompile(`;|&&`)
	// A failure handed to || is kept only if what follows fails again.
	reraise    = regexp.MustCompile(`\bexit\s+(?:[1-9]|\$\?)|\bfalse\b|\breturn\s+[1-9]`)
	conditions = set("if", "elif", "while", "until", "!")
	tests      = set("[", "[[", "test")
)

// Swallowed reports whether a script discards the failure of every command
// that runs reports true for: one followed by || and anything but a
// re-raise, or one that is the condition of an if, a while or a !. A script
// with no such command swallows nothing.
func Swallowed(script string, runs func(command string) bool) bool {
	found := false
	for _, line := range strings.Split(strings.ReplaceAll(script, "\\\n", " "), "\n") {
		start := 0
		for _, end := range append(statementEnd.FindAllStringIndex(line, -1), []int{len(line), len(line)}) {
			statement := line[start:end[0]]
			rest := line[start:]
			start = end[1]

			command, _, handled := strings.Cut(statement, "||")
			words := Words(command)
			conditional := false
			for len(words) > 0 && conditions[words[0]] {
				words, conditional = words[1:], true
			}
			if len(words) == 0 || strings.HasPrefix(words[0], "#") || Passive(words) || tests[words[0]] || !runs(strings.Join(words, " ")) {
				continue
			}
			found = true
			_, afterOr, _ := strings.Cut(rest, "||")
			if swallowed := conditional || handled && !reraise.MatchString(afterOr); !swallowed {
				return false
			}
		}
	}
	return found
}
