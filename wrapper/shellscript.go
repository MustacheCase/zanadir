package wrapper

import (
	"bytes"
	"regexp"
	"strings"

	"github.com/MustacheCase/zanadir/internal/repofs"
	"github.com/MustacheCase/zanadir/internal/shell"
)

var (
	errexitShebang = regexp.MustCompile(`^#!.*\s-[a-zA-Z]*e`)
	setFlags       = regexp.MustCompile(`^set\s+(.*)$`)
)

var shells = map[string]bool{"bash": true, "sh": true, "zsh": true, "dash": true, "ksh": true}

// conditionWords start a command whose failure the script itself handles,
// so errexit does not apply to it.
var conditionWords = map[string]bool{"if": true, "elif": true, "while": true, "until": true, "!": true}

// shellRun follows "bash [options] file" and "bash -c 'commands'".
func (e *Expander) shellRun(dir string, args []string) {
	errexit := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-c":
			if i+1 < len(args) {
				e.script(dir, strings.Trim(args[i+1], `"'`))
			}
			return
		case arg == "-o" || arg == "+o":
			if i+1 < len(args) {
				i++
				errexit = errexit || arg == "-o" && args[i] == "errexit"
			}
		case strings.HasPrefix(arg, "--"):
		case strings.HasPrefix(arg, "-"):
			errexit = errexit || strings.Contains(arg, "e")
		default:
			e.shellFile(dir, arg, errexit)
			return
		}
	}
}

// isScriptPath is a command that runs a shell script directly.
func isScriptPath(word string) bool {
	word = strings.Trim(word, `"'`)
	return strings.HasSuffix(word, ".sh") || strings.HasSuffix(word, ".bash")
}

// shellFile emits the commands of a script in the repository. Without
// errexit only the last command decides the script's status, so the others,
// conditions aside, are marked as unable to fail it.
func (e *Expander) shellFile(dir, name string, errexit bool) {
	name = strings.Trim(name, `"'`)
	if name == "" || strings.ContainsAny(name, "$`*?") {
		return
	}
	path, ok := e.within(dir, name)
	if !ok || !e.once("shell "+path) {
		return
	}
	data, err := repofs.ReadFile(e.root, path)
	if err != nil || bytes.IndexByte(data, 0) >= 0 {
		return
	}
	text := string(data)
	errexit = errexit || errexitShebang.MatchString(text)
	lines := shell.Lines(text)
	for i, line := range lines {
		switch words := shell.Words(line); {
		case len(words) == 2 && words[0] == "cd":
			if next, ok := e.within(dir, strings.Trim(words[1], `"'`)); ok {
				dir = next
			}
			continue
		case len(words) > 0 && words[0] == "set":
			errexit = setsErrexit(line, errexit)
		case len(words) > 0 && conditionWords[words[0]]:
			e.emit(dir, line)
			continue
		}
		e.ignored(!errexit && i < len(lines)-1, func() { e.emit(dir, line) })
	}
}

// setsErrexit applies a set command's -e, +e, -o errexit or +o errexit.
func setsErrexit(command string, errexit bool) bool {
	m := setFlags.FindStringSubmatch(strings.TrimSpace(command))
	if m == nil {
		return errexit
	}
	words := strings.Fields(m[1])
	for i, word := range words {
		switch {
		case (word == "-o" || word == "+o") && i+1 < len(words) && words[i+1] == "errexit":
			errexit = word == "-o"
		case strings.HasPrefix(word, "-") && !strings.HasPrefix(word, "--") && word != "-o" && strings.Contains(word, "e"):
			errexit = true
		case strings.HasPrefix(word, "+") && word != "+o" && strings.Contains(word, "e"):
			errexit = false
		}
	}
	return errexit
}
