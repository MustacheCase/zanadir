// Package shell reads the commands of a CI step's script without running it.
package shell

import (
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
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

// NormalizeCommand reduces the executable word to its unquoted basename.
// Arguments and assignments retain their quoting; no shell expansion is done.
func NormalizeCommand(command string) string {
	words := split(strings.TrimSpace(command), false)
	for i, word := range words {
		if word == "sudo" || conditions[word] || strings.Contains(word, "=") {
			continue
		}
		program := word
		if len(program) >= 2 && (program[0] == '\'' || program[0] == '"') && program[len(program)-1] == program[0] {
			program = program[1 : len(program)-1]
		}
		base := path.Base(program)
		// A dynamic basename or incomplete quoting cannot identify a tool.
		if program != "" && !strings.HasSuffix(program, "/") && !strings.ContainsAny(program, "`\"'") && !strings.ContainsAny(base, "$\\() \t\n") {
			words[i] = base
		}
		break
	}
	return strings.Join(words, " ")
}

// passiveCommands handle, fetch or look up a tool's files without running it:
// making a freshly built grype executable, or downloading its installer,
// scans nothing.
var passiveCommands = set("cat", "chmod", "chown", "cp", "curl", "echo", "file", "install", "ln", "ls",
	"mkdir", "mv", "printf", "readlink", "realpath", "rm", "stat", "tar", "touch", "type", "unzip", "wget", "which")

// A package manager installs a tool when one of its first two words says so:
// "pip install bandit", "uv tool install ruff", "go install …/gosec".
var (
	packageManagers = set("apk", "apt", "apt-get", "brew", "bun", "cargo", "choco", "composer", "conda", "dnf",
		"dotnet", "gem", "go", "mamba", "npm", "pip", "pip3", "pipx", "pnpm", "poetry", "scoop", "uv", "yarn", "yum")
	installing = set("add", "binstall", "ci", "download", "get", "i", "inject", "install", "require")
	// Asking a tool for its version or help runs no scan.
	selfDescribing = set("--help", "--version", "-V", "-h", "-version", "help", "version")
	// Through a package runner only the flags count: "pnpm changeset version" and
	// "npx lerna version" bump versions.
	selfDescribingFlags = set("--help", "--version", "-V", "-h", "-version")
	// Package runners that run the tool named after them ("npx eslint"). "run" is
	// never stripped after npm, pnpm or yarn: that runs a package.json script,
	// which is expanded and read on its own.
	runners = set("npx", "bunx", "pnpx", "uvx")
	// The verb a runner needs before the tool.
	runnerVerbs = map[string]map[string]bool{
		"bun": set("x"), "hatch": set("run"), "npm": set("exec"), "pdm": set("run"), "pipx": set("run"),
		"pnpm": set("exec", "dlx"), "poetry": set("run"), "uv": set("run"), "yarn": set("exec", "dlx"),
	}
	// Runners that take a bare tool name.
	bareRunners = set("pnpm", "yarn")
)

// runnerTool returns the words after a leading package runner and the runner's
// own flags ("npx --yes eslint --version" gives "eslint --version"), or nil
// when words do not start with a runner.
func runnerTool(words []string) []string {
	program, rest := filepath.Base(words[0]), words[1:]
	switch {
	case runners[program]:
	case program == "uv" && len(rest) > 1 && rest[0] == "tool" && rest[1] == "run":
		rest = rest[2:]
	case len(rest) > 0 && runnerVerbs[program][rest[0]]:
		rest = rest[1:]
	case bareRunners[program] && len(rest) > 0 && !strings.HasPrefix(rest[0], "-"):
	default:
		return nil
	}
	for len(rest) > 0 && strings.HasPrefix(rest[0], "-") {
		rest = rest[1:]
	}
	return rest
}

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
	if tool := runnerTool(words); len(tool) == 2 && selfDescribingFlags[tool[1]] {
		return true
	}
	program := filepath.Base(words[0])
	switch {
	case passiveCommands[program], program == "command" && len(words) > 1 && (words[1] == "-v" || words[1] == "-V"):
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
	reraise    = regexp.MustCompile(`\bexit\s+(?:[1-9]|"?\$(?:\?|\{?[A-Za-z_]))|\bfalse\b|\breturn\s+[1-9]`)
	conditions = set("if", "elif", "while", "until", "!")
	tests      = set("[", "[[", "test")
)

// Swallowed reports whether a script discards the failure of every command
// that runs reports true for: one followed by || and anything but a
// re-raise, or one that is the condition of an if, a while or a ! with no
// re-raise anywhere after it. A script with no such command swallows nothing.
func Swallowed(script string, runs func(command string) bool) bool {
	found := false
	for _, s := range statements(script) {
		if runs(s.command) {
			if !s.swallowed {
				return false
			}
			found = true
		}
	}
	return found
}

type statement struct {
	command   string
	swallowed bool
}

// statements returns the script's statements that run something, each with
// whether the script discards its failure there.
func statements(script string) []statement {
	var found []statement
	var owners []int
	var ends []int
	var relaxed []bool
	var raw []string
	errexit := true
	text := strings.ReplaceAll(script, "\\\n", " ")
	textReraise, lineStart := lastReraise(text), 0
	for _, line := range strings.Split(text, "\n") {
		lineReraise, start, or := lastReraise(line), 0, -1
		for _, end := range append(statementEnd.FindAllStringIndex(line, -1), []int{len(line), len(line)}) {
			if or < start {
				or = len(line)
				if i := strings.Index(line[start:], "||"); i >= 0 {
					or = start + i
				}
			}
			handled := or < end[0]
			command := line[start:min(or, end[0])]
			later := lineStart + end[0]
			start = end[1]

			inner, masked, outer := substitutions(command)
			words := Words(outer)
			if trimmed := strings.TrimSpace(command); trimmed == "" || strings.HasPrefix(trimmed, "#") {
				continue
			}
			raw, ends, relaxed = append(raw, command), append(ends, later), append(relaxed, !errexit)
			errexit = setsErrexit(words, errexit)
			conditional := false
			for len(words) > 0 && conditions[words[0]] {
				words, conditional = words[1:], true
			}
			swallowed := conditional && textReraise < later || handled && lineReraise < or+2
			for _, script := range inner {
				for _, s := range statements(script) {
					found, owners = append(found, statement{s.command, s.swallowed || masked || swallowed}), append(owners, len(raw)-1)
				}
			}
			if len(words) == 0 || Passive(words) || tests[words[0]] {
				continue
			}
			found, owners = append(found, statement{strings.Join(words, " "), swallowed}), append(owners, len(raw)-1)
		}
		lineStart += len(line) + 1
	}
	// Under set +e a failure is lost when something runs after it and nothing
	// reads its status, or when a case on the status lets a failing code by.
	for i, owner := range owners {
		after := text[ends[owner]:]
		if relaxed[owner] && (runsLater(raw[owner+1:]) && !readsStatus.MatchString(after) || acceptsFailure(after)) {
			found[i].swallowed = true
		}
	}
	return found
}

var readsStatus = regexp.MustCompile(`\$\?|PIPESTATUS`)

// closers end a block or name a case branch, running nothing themselves.
var closers = set("fi", "done", "esac", "then", "else", "do", "{", "}")

func runsLater(commands []string) bool {
	return slices.ContainsFunc(commands, func(command string) bool {
		words := Words(command)
		return len(words) > 0 && !closers[words[0]] && !strings.HasSuffix(words[0], ")")
	})
}

// EndsRelaxed reports whether a script leaves set +e on at its end.
func EndsRelaxed(script string) bool {
	errexit := true
	for _, command := range Commands(script) {
		errexit = setsErrexit(Words(command), errexit)
	}
	return !errexit
}

// setsErrexit returns whether bash -e is on after words run.
func setsErrexit(words []string, errexit bool) bool {
	if len(words) == 0 || words[0] != "set" {
		return errexit
	}
	for i, word := range words[1:] {
		switch {
		case (word == "-o" || word == "+o") && i+2 < len(words) && words[i+2] == "errexit":
			errexit = word == "-o"
		case len(word) > 1 && (word[0] == '-' || word[0] == '+') && word[1] != 'o' && strings.Contains(word, "e"):
			errexit = word[0] == '-'
		}
	}
	return errexit
}

var statusCase = regexp.MustCompile(`(?s)\bcase\s+"?\$\{?(\?|[A-Za-z_]\w*)\}?"?\s+in\b(.*?)\besac\b`)

// acceptsFailure reports whether a case on the exit status lets a non-zero
// code through, as case $rc in 0|1) does for a scanner's findings.
func acceptsFailure(text string) bool {
	m := statusCase.FindStringSubmatch(text)
	if m == nil || m[1] != "?" && !strings.Contains(text, m[1]+"=$?") {
		return false
	}
	for _, branch := range strings.Split(m[2], ";;") {
		pattern, body, ok := strings.Cut(branch, ")")
		if !ok {
			continue
		}
		for _, code := range strings.Split(strings.Trim(strings.TrimSpace(pattern), "("), "|") {
			if n, err := strconv.Atoi(strings.TrimSpace(code)); err == nil && n != 0 && !reraise.MatchString(body) {
				return true
			}
		}
	}
	return false
}

// Substitutions returns the scripts a command runs in $(...) outside its
// program word, and whether it hides their status (all but OUT=$(tool) do).
func Substitutions(command string) (inner []string, masked bool) {
	inner, masked, _ = substitutions(command)
	return inner, masked
}

// substitutions also returns the command with those scripts emptied.
func substitutions(command string) (inner []string, masked bool, outer string) {
	words := split(strings.TrimSpace(command), false)
	program := false
	for i, word := range words {
		assignment := !program && strings.Contains(word, "=")
		if !assignment && !program {
			program = true
			continue
		}
		var scripts []string
		scripts, words[i] = substituted(word)
		inner = append(inner, scripts...)
	}
	return inner, program, strings.Join(words, " ")
}

// substituted skips one-word scripts: $(pwd), or Azure's $(Build.Reason) macro.
func substituted(word string) ([]string, string) {
	var scripts []string
	var kept strings.Builder
	quoted := false
	for i := 0; i < len(word); i++ {
		c := word[i]
		if c == '\'' {
			quoted = !quoted
		}
		if quoted || c != '$' || i+1 >= len(word) || word[i+1] != '(' || i+2 < len(word) && word[i+2] == '(' {
			kept.WriteByte(c)
			continue
		}
		depth, end := 0, len(word)
		for j := i + 1; j < len(word); j++ {
			if word[j] == '(' {
				depth++
			} else if word[j] == ')' {
				if depth--; depth == 0 {
					end = j
					break
				}
			}
		}
		if script := word[i+2 : end]; len(strings.Fields(script)) > 1 {
			scripts = append(scripts, script)
			kept.WriteString("$()")
		} else {
			kept.WriteString(word[i:min(end+1, len(word))])
		}
		i = end
	}
	return scripts, kept.String()
}

// lastReraise returns where the last re-raise in text starts, or -1.
func lastReraise(text string) int {
	matches := reraise.FindAllStringIndex(text, -1)
	if len(matches) == 0 {
		return -1
	}
	return matches[len(matches)-1][0]
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
	"--message", "--author", "--label", "--target", "--repo")

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
	return SwallowedCommands(script)(command)
}

// SwallowedCommands reads script once and reports, for any of its Commands,
// whether the script discards that command's failure wherever it appears.
func SwallowedCommands(script string) func(command string) bool {
	swallowed := map[string]bool{}
	for _, s := range statements(script) {
		all, seen := swallowed[s.command]
		swallowed[s.command] = s.swallowed && (all || !seen)
	}
	return func(command string) bool {
		words := Words(command)
		for len(words) > 0 && conditions[words[0]] {
			words = words[1:]
		}
		return swallowed[strings.Join(words, " ")]
	}
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
