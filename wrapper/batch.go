package wrapper

import (
	"bytes"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/MustacheCase/zanadir/internal/repofs"
)

var (
	batchVar   = regexp.MustCompile(`%%|%~?[0-9*]|%[A-Za-z_][\w.-]*%`)
	batchDo    = regexp.MustCompile(`(?i)\s+do\s+`)
	batchEcho  = regexp.MustCompile(`^(\d?>&?\d?\s*)?echo([\s.:(/]|$)`)
	compareOps = map[string]bool{"==": true, "equ": true, "neq": true, "lss": true, "leq": true, "gtr": true, "geq": true}
	launchers  = map[string]bool{"gradlew.bat": true, "mvnw.cmd": true}
)

type batchFile struct {
	lines  []string
	labels map[string]int
}

type batchStep struct {
	dir, command string
	fails        bool
}

// batchRun walks a batch file as cmd would for one set of arguments: known
// conditions and gotos are followed, and an unknown branch runs without its
// jumps or exits.
type batchRun struct {
	e      *Expander
	file   *batchFile
	args   []string
	vars   map[string]string
	dir    string
	dirs   []string
	jumped map[string]bool
	blocks []bool
	steps  []batchStep
}

func isBatchPath(word string) bool {
	word = strings.ToLower(strings.Trim(word, `"'`))
	return strings.HasSuffix(word, ".bat") || strings.HasSuffix(word, ".cmd")
}

// batchFile emits the commands a batch file runs. cmd does not stop at a
// failure, so a command only fails the file when an errorlevel check, an
// "|| exit /b" or "|| goto" follows it, or the file ends on it.
func (e *Expander) batchFile(dir, name string, args []string) {
	name = strings.ReplaceAll(strings.Trim(name, `"'`), `\`, "/")
	if strings.ContainsAny(name, "$%`*?") || launchers[strings.ToLower(filepath.Base(name))] {
		return
	}
	path, ok := e.within(dir, name)
	if !ok || !e.once("batch "+path+" "+strings.Join(args, " ")) {
		return
	}
	data, err := repofs.ReadFile(e.root, path)
	if err != nil || bytes.IndexByte(data, 0) >= 0 {
		return
	}
	r := &batchRun{e: e, file: parseBatch(string(data)), args: args, vars: map[string]string{}, dir: dir, jumped: map[string]bool{}}
	if r.walk(0, 0) {
		r.check()
	}
	for _, step := range r.steps {
		e.ignored(!step.fails, func() { e.emit(step.dir, step.command) })
	}
}

func parseBatch(text string) *batchFile {
	f := &batchFile{labels: map[string]int{}}
	held := ""
	for _, line := range strings.Split(text, "\n") {
		line, held = strings.TrimSpace(held+strings.TrimSpace(line)), ""
		if strings.HasSuffix(line, "^") {
			held = strings.TrimSuffix(line, "^")
			continue
		}
		switch {
		case line == "" || strings.HasPrefix(line, "::"):
		case strings.HasPrefix(line, ":"):
			if _, ok := f.labels[labelName(line)]; !ok {
				f.labels[labelName(line)] = len(f.lines)
			}
		default:
			f.lines = append(f.lines, strings.ReplaceAll(line, `\`, "/"))
		}
	}
	return f
}

func labelName(word string) string {
	fields := strings.Fields(strings.TrimPrefix(word, ":"))
	if len(fields) == 0 {
		return ""
	}
	return strings.ToLower(strings.TrimRight(fields[0], ")"))
}

// walk runs lines from i and reports whether the last command's status is
// the file's.
func (r *batchRun) walk(i, depth int) bool {
	saved := r.blocks
	r.blocks = nil
	defer func() { r.blocks = saved }()
	for i < len(r.file.lines) && len(r.steps) < maxLines {
		line := r.expand(r.file.lines[i])
		switch {
		case strings.HasPrefix(line, ")"):
			i = r.close(i, line)
		case line == "(":
			r.blocks = append(r.blocks, true)
			i++
		default:
			var fails bool
			if i, fails = r.statement(line, i, depth); i < 0 {
				return fails
			}
		}
	}
	return true
}

// statement runs one statement on line i and returns the next line, or -1
// when the file stops there and whether its last command decides its status.
func (r *batchRun) statement(s string, i, depth int) (int, bool) {
	s = strings.TrimSpace(strings.TrimPrefix(s, "@"))
	words := strings.Fields(s)
	if len(words) == 0 {
		return i + 1, false
	}
	if parts := chained(s); len(parts) > 1 && !strings.EqualFold(words[0], "if") && !strings.EqualFold(words[0], "for") {
		for _, part := range parts {
			if next, fails := r.statement(part, i, depth); next != i+1 {
				return next, fails
			}
		}
		return i + 1, false
	}
	switch command := strings.ToLower(words[0]); {
	case command == "rem" || command == "setlocal" || command == "endlocal" || command == "echo" && len(words) == 2 && (strings.EqualFold(words[1], "off") || strings.EqualFold(words[1], "on")):
	case command == "goto" && r.control():
		target := ""
		if len(words) > 1 {
			target = labelName(words[1])
		}
		next, ok := r.file.labels[target]
		if target == "eof" || !ok || r.jumped[target] {
			return -1, target == "eof" || ok
		}
		r.jumped[target] = true
		return next, false
	case command == "exit" && r.control():
		code := slices.DeleteFunc(words[1:], func(w string) bool { return strings.EqualFold(w, "/b") })
		return -1, len(code) == 0 || strings.Contains(strings.ToLower(code[0]), "errorlevel")
	case command == "call":
		if len(words) < 2 {
			break
		}
		if strings.HasPrefix(words[1], ":") {
			if next, ok := r.file.labels[labelName(words[1])]; ok && depth < maxDepth {
				r.walk(next, depth+1)
			}
		} else if rest := strings.TrimSpace(s[len(words[0]):]); strings.EqualFold(words[1], "set") {
			r.set(rest)
		} else {
			r.record(rest)
		}
	case command == "set":
		r.set(s)
	case command == "if":
		return r.condition(words, i, depth)
	case command == "for":
		if at := batchDo.FindStringIndex(s); at != nil {
			r.maybe(strings.TrimSpace(s[at[1]:]), i, depth)
		}
	case command == "cd" || command == "chdir" || command == "pushd" || command == "popd":
		r.cd(command, words[1:])
	case command != "goto" && command != "exit":
		r.record(s)
		if isBatchPath(words[0]) && r.control() {
			return -1, true
		}
	}
	return i + 1, false
}

// condition runs an if, following it only when its outcome is known. An
// errorlevel test checks the command before it: its success branch is
// followed and its failure branch skipped.
func (r *batchRun) condition(words []string, i, depth int) (int, bool) {
	k, fold, negate := 1, false, false
	if k < len(words) && strings.EqualFold(words[k], "/i") {
		k, fold = k+1, true
	}
	if k < len(words) && strings.EqualFold(words[k], "not") {
		k, negate = k+1, true
	}
	known, truth, errorlevel, success := false, false, false, false
	switch {
	case k+1 < len(words) && (strings.EqualFold(words[k], "errorlevel") || strings.EqualFold(words[k], "exist") || strings.EqualFold(words[k], "defined")):
		errorlevel = strings.EqualFold(words[k], "errorlevel")
		k += 2
	case k < len(words) && strings.Contains(words[k], "=="):
		a, b, _ := strings.Cut(words[k], "==")
		known, truth, errorlevel, success = compare(a, b, "==", fold)
		k++
	case k+2 < len(words) && compareOps[strings.ToLower(words[k+1])]:
		known, truth, errorlevel, success = compare(words[k], words[k+2], strings.ToLower(words[k+1]), fold)
		k += 3
	default:
		return i + 1, false
	}
	truth, success = truth != negate, success != negate
	rest := strings.Join(words[k:], " ")
	block := rest == "("
	switch {
	case errorlevel && success && (block || jumps(rest)):
		r.check()
		if !block {
			return r.statement(rest, i, depth)
		}
		r.blocks = append(r.blocks, true)
	case errorlevel && !success && (block || raises(rest)):
		r.check()
		if block {
			return r.skip(i), false
		}
	case known && !truth:
		if block {
			return r.skip(i), false
		}
	case block:
		r.blocks = append(r.blocks, known)
	case known:
		return r.statement(rest, i, depth)
	default:
		r.maybe(rest, i, depth)
	}
	return i + 1, false
}

// compare evaluates a == b or a op b, or for an errorlevel test reports
// whether it holds when the command succeeded.
func compare(a, b, op string, fold bool) (known, truth, errorlevel, success bool) {
	if strings.Contains(strings.ToLower(a), "errorlevel") {
		a, b = b, a
	}
	if strings.Contains(strings.ToLower(b), "errorlevel") {
		zero := strings.Trim(a, `"`) == "0"
		return false, false, true, op == "lss" || op == "leq" || zero && (op == "==" || op == "equ")
	}
	if strings.ContainsAny(a+b, "%!") || op != "==" && op != "equ" && op != "neq" {
		return false, false, false, false
	}
	a, b = strings.Trim(a, `"`), strings.Trim(b, `"`)
	equal := a == b || fold && strings.EqualFold(a, b)
	return true, equal != (op == "neq"), false, false
}

// maybe runs a statement that may not run at all, so it cannot jump.
func (r *batchRun) maybe(s string, i, depth int) {
	if s == "(" {
		r.blocks = append(r.blocks, false)
		return
	}
	r.blocks = append(r.blocks, false)
	r.statement(s, i, depth)
	r.blocks = r.blocks[:len(r.blocks)-1]
}

func (r *batchRun) control() bool {
	return !slices.Contains(r.blocks, false)
}

// close ends a block, skipping the else of a branch that was taken.
func (r *batchRun) close(i int, line string) int {
	n := len(r.blocks)
	if n == 0 {
		return i + 1
	}
	taken := r.blocks[n-1]
	r.blocks = r.blocks[:n-1]
	switch {
	case !strings.HasSuffix(line, "("):
		return i + 1
	case taken:
		return r.skip(i)
	}
	r.blocks = append(r.blocks, false)
	return i + 1
}

// skip passes over the block opened on line i, entering its else if any.
func (r *batchRun) skip(i int) int {
	depth := 1
	for j := i + 1; j < len(r.file.lines); j++ {
		line := r.file.lines[j]
		if strings.HasPrefix(line, ")") {
			if depth--; depth == 0 {
				if strings.HasSuffix(line, "(") {
					r.blocks = append(r.blocks, true)
				}
				return j + 1
			}
		}
		if strings.HasSuffix(line, "(") {
			depth++
		}
	}
	return len(r.file.lines)
}

// jumps reports whether a statement is a goto or an exit, and raises whether
// it also passes a failure on, as all but an exit with code 0 do.
func jumps(s string) bool {
	words := strings.Fields(strings.Trim(s, "() "))
	return len(words) > 0 && (strings.EqualFold(words[0], "goto") || strings.EqualFold(words[0], "exit"))
}

func raises(s string) bool {
	words := strings.Fields(strings.Trim(s, "() "))
	return jumps(s) && (!strings.EqualFold(words[0], "exit") || words[len(words)-1] != "0")
}

// chained splits s at each & that is not part of &&, a redirection or ^&.
func chained(s string) []string {
	var parts []string
	quoted, start := false, 0
	for i := 0; i < len(s); i++ {
		switch {
		case s[i] == '"':
			quoted = !quoted
		case s[i] == '&' && !quoted && (i == 0 || !strings.ContainsRune(">&^", rune(s[i-1]))) && (i+1 == len(s) || s[i+1] != '&'):
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	return append(parts, s[start:])
}

// record adds a command. An echo's prose would read as shell quoting, so it
// is kept only as a step that sets the status.
func (r *batchRun) record(command string) {
	fails := false
	if at := strings.LastIndex(command, "||"); at >= 0 && raises(command[at+2:]) {
		command, fails = command[:at], true
	}
	if lower := strings.ToLower(command); batchEcho.MatchString(lower) && !strings.Contains(lower, "|") {
		command = ""
	}
	r.steps = append(r.steps, batchStep{dir: r.dir, command: strings.TrimSpace(command), fails: fails})
}

// check marks the last command as one whose failure fails the file.
func (r *batchRun) check() {
	if n := len(r.steps); n > 0 {
		r.steps[n-1].fails = true
	}
}

func (r *batchRun) set(s string) {
	rest := strings.TrimSpace(s[len("set"):])
	if strings.HasPrefix(rest, `"`) {
		rest = rest[1:]
		if end := strings.LastIndex(rest, `"`); end >= 0 {
			rest = rest[:end]
		}
	}
	name, value, ok := strings.Cut(rest, "=")
	if name = strings.TrimSpace(name); ok && name != "" && !strings.HasPrefix(name, "/") && !strings.Contains(value, "%%") {
		r.vars[strings.ToLower(name)] = value
	}
}

func (r *batchRun) cd(command string, args []string) {
	if command == "popd" {
		if n := len(r.dirs); n > 0 {
			r.dir, r.dirs = r.dirs[n-1], r.dirs[:n-1]
		}
		return
	}
	args = slices.DeleteFunc(args, func(arg string) bool { return strings.EqualFold(arg, "/d") })
	if len(args) == 0 {
		return
	}
	if command == "pushd" {
		r.dirs = append(r.dirs, r.dir)
	}
	if target := strings.Trim(args[0], `"`); !strings.Contains(target, "%") {
		if next, ok := r.e.within(r.dir, target); ok {
			r.dir = next
		}
	}
}

// expand substitutes the file's arguments and the variables it set, leaving
// any other %VAR% for the environment.
func (r *batchRun) expand(line string) string {
	return batchVar.ReplaceAllStringFunc(line, func(m string) string {
		switch last := m[len(m)-1]; {
		case m == "%%":
			return m
		case last == '*':
			return strings.Join(r.args, " ")
		case last >= '0' && last <= '9':
			n := int(last - '0')
			switch {
			case n == 0:
				return m
			case n > len(r.args):
				return ""
			case m[1] == '~':
				return strings.Trim(r.args[n-1], `"`)
			}
			return r.args[n-1]
		}
		if value, ok := r.vars[strings.ToLower(m[1:len(m)-1])]; ok {
			return value
		}
		return m
	})
}
