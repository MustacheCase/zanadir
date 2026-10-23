// Package shell reads the commands of a CI step's script without running it.
package shell

import (
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// Commands splits a script into commands, dropping comments: they name tools
// without running them.
func Commands(script string) []string {
	var commands []string
	for _, command := range split(strings.ReplaceAll(script, "\\\n", " "), true) {
		if command = strings.TrimSpace(command); command != "" {
			commands = append(commands, command)
		}
	}
	return commands
}

var heredoc = regexp.MustCompile(`<<-?\s*['"]?([A-Za-z_][A-Za-z0-9_]*)['"]?`)

// split cuts text at its top-level separators - between commands, or with
// words set at whitespace - leaving quotes, $(...) and backticks whole. A #
// starting a word begins a comment that runs to the end of the line, and a
// heredoc's body is text, not commands.
func split(text string, commands bool) []string {
	var parts []string
	var current strings.Builder
	cut := func() {
		parts = append(parts, current.String())
		current.Reset()
	}
	quote, depth := byte(0), 0
	for i := 0; i < len(text); i++ {
		c := text[i]
		top := quote == 0 && depth == 0
		switch {
		case c == '\\' && quote != '\'' && i+1 < len(text):
			current.WriteByte(c)
			i++
			c = text[i]
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '\'' || c == '"' || c == '`':
			quote = c
		case c == '$' && i+1 < len(text) && text[i+1] == '(':
			depth++
		case c == ')' && depth > 0:
			depth--
		case top && c == '#' && (i == 0 || strings.ContainsRune(" \t\n;|&", rune(text[i-1]))):
			for i+1 < len(text) && text[i+1] != '\n' {
				i++
			}
			continue
		case top && commands && c == '\n':
			if body := heredoc.FindStringSubmatch(current.String()); body != nil {
				i = skipHeredoc(text, i, body[1])
			}
			cut()
			continue
		case top && commands && (c == ';' || c == '|' || c == '&' && i+1 < len(text) && text[i+1] == '&'):
			if i+1 < len(text) && (text[i+1] == '|' || text[i+1] == '&') && c != ';' {
				i++
			}
			cut()
			continue
		case top && !commands && (c == ' ' || c == '\t'):
			if current.Len() > 0 {
				cut()
			}
			continue
		}
		current.WriteByte(c)
	}
	if current.Len() > 0 || commands {
		cut()
	}
	return parts
}

// Words returns a command's program and arguments, past sudo and VAR=value.
func Words(command string) []string {
	words := split(strings.TrimSpace(command), false)
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
	for len(words) > 0 && conditions[words[0]] {
		words = words[1:]
	}
	if len(words) == 0 || tests[words[0]] {
		return true
	}
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
// re-raise, or one that is the condition of an if, a while or a ! with no
// re-raise anywhere after it. A script with no such command swallows nothing.
func Swallowed(script string, runs func(command string) bool) bool {
	found := false
	lines := strings.Split(strings.ReplaceAll(script, "\\\n", " "), "\n")
	for i, line := range lines {
		start := 0
		for _, end := range append(statementEnd.FindAllStringIndex(line, -1), []int{len(line), len(line)}) {
			statement := line[start:end[0]]
			rest := line[start:]
			later := line[end[0]:] + "\n" + strings.Join(lines[i+1:], "\n")
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
			if swallowed := conditional && !reraise.MatchString(later) || handled && !reraise.MatchString(afterOr); !swallowed {
				return false
			}
		}
	}
	return found
}

// skipHeredoc returns where a heredoc's body ends: the end of the line that
// holds only its delimiter, or of the text.
func skipHeredoc(text string, newline int, delimiter string) int {
	for start := newline + 1; start < len(text); {
		end := strings.IndexByte(text[start:], '\n')
		if end < 0 {
			end = len(text) - start
		}
		if strings.TrimSpace(text[start:start+end]) == delimiter {
			return start + end
		}
		start += end + 1
	}
	return len(text)
}

// namingFlags take a value that names something - a binary, a package, an
// image tag - rather than running it. -m is not one: "python -m pyright"
// runs what it names, and a commit message is quoted anyway.
var namingFlags = set("--bin", "--package", "--chart", "--charts", "--tag", "-t", "--name", "--title",
	"--message", "--author", "--label", "--target")

var shells = set("bash", "sh", "zsh", "dash", "pwsh", "powershell", "cmd")

// Invoked keeps the words that can name what a command runs. A quoted
// argument to a flag is text, a commit message or a licence holder, unless
// the command is a shell running it as a script; a naming flag's value
// names a thing rather than running it.
func Invoked(words []string) []string {
	if len(words) == 0 {
		return nil
	}
	shell := shells[filepath.Base(words[0])]
	kept := []string{words[0]}
	for i := 1; i < len(words); i++ {
		flag := strings.HasPrefix(words[i-1], "-")
		quoted := strings.ContainsAny(words[i][:1], `"'`)
		if flag && (namingFlags[words[i-1]] || quoted && !shell) {
			continue
		}
		kept = append(kept, words[i])
	}
	return kept
}

// SwallowedCommand reports whether script, one of whose Commands is command,
// discards that command's failure wherever it appears.
func SwallowedCommand(script, command string) bool {
	words := Words(command)
	for len(words) > 0 && conditions[words[0]] {
		words = words[1:]
	}
	target := strings.Join(words, " ")
	return Swallowed(script, func(c string) bool { return c == target })
}

// Lines returns a script's command lines as written, || and all: line
// continuations joined, and blank lines, comments and heredoc bodies left
// out.
func Lines(script string) []string {
	text := strings.ReplaceAll(script, "\\\n", " ")
	var lines []string
	for i := 0; i < len(text); {
		end := strings.IndexByte(text[i:], '\n')
		if end < 0 {
			end = len(text) - i
		}
		line := text[i : i+end]
		next := i + end + 1
		if m := heredoc.FindStringSubmatch(line); m != nil {
			next = skipHeredoc(text, i+end, m[1]) + 1
		}
		if trimmed := strings.TrimSpace(line); trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			lines = append(lines, trimmed)
		}
		i = next
	}
	return lines
}
