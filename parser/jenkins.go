package parser

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/MustacheCase/zanadir/internal/repofs"
	"github.com/MustacheCase/zanadir/models"
)

// JenkinsParser reads Jenkinsfiles.
type JenkinsParser struct{}

const jenkinsFile = "Jenkinsfile"

// shellSteps run a command, which the rules match like a run: step.
var shellSteps = map[string]bool{"sh": true, "bat": true, "powershell": true, "pwsh": true}

// groovyBlocks are keywords whose block always runs, or runs in a way the
// file cannot narrow down, so their steps keep the surrounding context.
var groovyBlocks = map[string]bool{"try": true, "catch": true, "finally": true, "for": true, "while": true, "switch": true}

// Exists reports whether root has a Jenkinsfile.
func (j *JenkinsParser) Exists(root string) bool {
	return len(jenkinsFiles(root)) > 0
}

// jenkinsFiles is the Jenkinsfile at the root and variants such as
// Jenkinsfile.release beside it.
func jenkinsFiles(root string) []string {
	matches, _ := filepath.Glob(filepath.Join(root, jenkinsFile+"*"))
	var files []string
	for _, path := range matches {
		name := filepath.Base(path)
		if (name == jenkinsFile || strings.HasPrefix(name, jenkinsFile+".")) && !strings.HasSuffix(name, ".md") {
			if info, err := os.Stat(path); err == nil && !info.IsDir() {
				files = append(files, path)
			}
		}
	}
	return files
}

// Parse reads each Jenkinsfile as Groovy tokens rather than evaluating it.
// The file holds no trigger: which branches and pull requests build is set on
// the Jenkins job, so every control grades unknown unless the file itself
// weakens it.
func (j *JenkinsParser) Parse(root string) ([]*models.Artifact, error) {
	var artifacts []*models.Artifact
	for _, path := range jenkinsFiles(root) {
		data, err := repofs.ReadFile(root, path)
		if err != nil {
			artifacts = append(artifacts, unreadable(path, err))
			continue
		}
		w := &jenkinsWalker{tokens: groovyTokens(string(data))}
		artifacts = append(artifacts, &models.Artifact{
			Name:     "Jenkins",
			Jobs:     w.block(jenkinsContext{}),
			Location: path,
		})
	}
	return artifacts, nil
}

// jenkinsContext is what a step inherits from the blocks around it.
type jenkinsContext struct {
	stage           string
	condition       string
	continueOnError bool
	// unsure is a wrapper whose effect on the build cannot be read, such as
	// catchError marking it UNSTABLE.
	unsure string
}

type jenkinsWalker struct {
	tokens []groovyToken
	pos    int
}

func (w *jenkinsWalker) peek(offset int) groovyToken {
	if i := w.pos + offset; i >= 0 && i < len(w.tokens) {
		return w.tokens[i]
	}
	return groovyToken{}
}

func (w *jenkinsWalker) next() groovyToken {
	t := w.peek(0)
	w.pos++
	return t
}

// block reads statements until the closing brace of the current block.
func (w *jenkinsWalker) block(ctx jenkinsContext) []*models.Job {
	var jobs []*models.Job
	for w.pos < len(w.tokens) {
		t := w.next()
		switch {
		case t.is("}"):
			return jobs
		case t.is("{"):
			jobs = append(jobs, w.block(ctx)...)
		case t.kind == groovyIdent && !w.peek(-2).is("."):
			// A method called on a value, like url.trim(), is not a step.
			jobs = append(jobs, w.statement(t.text, &ctx)...)
		}
	}
	return jobs
}

// statement reads what follows an identifier. ctx belongs to the enclosing
// block: a when: block narrows every step after it in the same stage.
func (w *jenkinsWalker) statement(name string, ctx *jenkinsContext) []*models.Job {
	if groovyBlocks[name] {
		w.skipParens()
		return w.closure(*ctx)
	}
	switch name {
	case "when":
		if w.peek(0).is("{") {
			w.next()
			ctx.condition = joinConditions(ctx.condition, "when { "+w.skipBlock()+" }")
		}
		return nil
	case "if":
		inner := *ctx
		inner.condition = joinConditions(ctx.condition, "if ("+w.skipParens()+")")
		jobs := w.closure(inner)
		if w.peek(0).is("else") {
			w.next()
			inner.condition = joinConditions(ctx.condition, "else")
			jobs = append(jobs, w.closure(inner)...)
		}
		return jobs
	}

	args, hasArgs := w.args()
	inner := *ctx
	switch name {
	case "stage":
		inner.stage = args.first()
	case "catchError":
		switch result, set := args.named["buildResult"]; {
		case !set || strings.EqualFold(result, "FAILURE"):
		case strings.EqualFold(result, "SUCCESS") || result == "null":
			inner.continueOnError = true
		default:
			inner.unsure = "catchError(buildResult: " + result + ")"
		}
	case "warnError":
		inner.unsure = "warnError"
	}

	var jobs []*models.Job
	if hasArgs {
		if job := jenkinsStep(name, args, ctx); job != nil {
			jobs = append(jobs, job)
		}
	}
	return append(jobs, w.closure(inner)...)
}

// closure reads a block passed to the call just read, if there is one.
func (w *jenkinsWalker) closure(ctx jenkinsContext) []*models.Job {
	if !w.peek(0).is("{") {
		return nil
	}
	w.next()
	return w.block(ctx)
}

func jenkinsStep(name string, args groovyArgs, ctx *jenkinsContext) *models.Job {
	if name == "stage" || name == "catchError" || name == "warnError" {
		return nil
	}
	job := &models.Job{
		Name:            ctx.stage,
		ContinueOnError: ctx.continueOnError,
		If:              ctx.unsure,
		JobIf:           ctx.condition,
	}
	if !shellSteps[name] {
		job.Package, job.With = name, args.named
		return job
	}

	job.Run = args.named["script"]
	if job.Run == "" {
		job.Run = args.first()
	}
	if job.Run == "" {
		return nil
	}
	// The exit code is handed back instead of failing the build; what the
	// script does with it next cannot be read.
	if strings.EqualFold(args.named["returnStatus"], "true") {
		job.If = joinConditions(job.If, "returnStatus: true")
	}
	return job
}

// groovyArgs are a call's arguments: positional strings and named scalars.
type groovyArgs struct {
	positional []string
	named      map[string]string
}

func (a groovyArgs) first() string {
	if len(a.positional) == 0 {
		return ""
	}
	return a.positional[0]
}

// args reads a call's arguments in any of Groovy's forms: in parentheses, a
// bare string (sh 'make'), or bare named arguments (dependencyCheck odc: 'x').
func (w *jenkinsWalker) args() (groovyArgs, bool) {
	args := groovyArgs{named: map[string]string{}}
	switch {
	case w.peek(0).is("("):
		w.next()
		for !w.peek(0).is(")") && w.pos < len(w.tokens) {
			w.argument(&args, true)
			if w.peek(0).is(",") {
				w.next()
			}
		}
		w.next()
	case w.peek(0).kind == groovyString:
		args.positional = append(args.positional, w.next().text)
	case w.peek(0).kind == groovyIdent && w.peek(1).is(":"):
		w.argument(&args, false)
		for w.peek(0).is(",") {
			w.next()
			w.argument(&args, false)
		}
	default:
		return args, false
	}
	return args, true
}

// argument reads one argument. A value that is not a plain scalar - a list, a
// map, a closure, an expression - is skipped whole. Inside parentheses an
// argument runs to the next comma; without them it is a single value, so the
// statement after it is not swallowed.
func (w *jenkinsWalker) argument(args *groovyArgs, inParens bool) {
	key := ""
	if w.peek(0).kind == groovyIdent && w.peek(1).is(":") {
		key = w.next().text
		w.next()
	}

	value, scalar := "", false
	if t := w.peek(0); t.kind != groovyPunct && !w.peek(1).is("(") && !w.peek(1).is(".") {
		value, scalar = w.next().text, true
	}
	if inParens {
		for depth := 0; w.pos < len(w.tokens) && !w.argumentEnds(depth); w.next() {
			switch t := w.peek(0); {
			case t.is("(") || t.is("[") || t.is("{"):
				depth++
			case t.is(")") || t.is("]") || t.is("}"):
				depth--
			}
			scalar = false
		}
	} else if !scalar {
		w.skipValue()
	}

	switch {
	case !scalar:
	case key != "":
		args.named[key] = value
	default:
		args.positional = append(args.positional, value)
	}
}

// argumentEnds reports whether the next token closes an argument inside
// parentheses, at this nesting depth.
func (w *jenkinsWalker) argumentEnds(depth int) bool {
	return depth == 0 && (w.peek(0).is(",") || w.peek(0).is(")"))
}

// skipValue consumes one value that is not a scalar: a bracketed literal, or a
// call such as credentials('id') or env.HOME.
func (w *jenkinsWalker) skipValue() {
	for w.peek(0).kind == groovyIdent && w.peek(1).is(".") {
		w.pos += 2
	}
	if w.peek(0).kind == groovyIdent {
		w.next()
	}
	for _, pair := range [][2]string{{"(", ")"}, {"[", "]"}, {"{", "}"}} {
		if w.peek(0).is(pair[0]) {
			w.skipBalanced(pair[0], pair[1])
			return
		}
	}
}

// skipParens consumes a parenthesised expression and returns its text.
func (w *jenkinsWalker) skipParens() string {
	if !w.peek(0).is("(") {
		return ""
	}
	return w.skipBalanced("(", ")")
}

// skipBlock consumes the rest of a block whose brace was already read.
func (w *jenkinsWalker) skipBlock() string {
	w.pos--
	return w.skipBalanced("{", "}")
}

func (w *jenkinsWalker) skipBalanced(open, close string) string {
	w.next()
	var text []string
	for depth := 1; w.pos < len(w.tokens); {
		t := w.next()
		switch {
		case t.is(open):
			depth++
		case t.is(close):
			depth--
		}
		if depth == 0 {
			break
		}
		text = append(text, t.text)
	}
	return strings.Join(text, " ")
}

type groovyKind int

const (
	groovyPunct groovyKind = iota
	groovyIdent
	groovyString
	groovyOther
)

type groovyToken struct {
	kind groovyKind
	text string
}

func (t groovyToken) is(text string) bool {
	return t.kind != groovyString && t.text == text
}

// groovyTokens splits Groovy into identifiers, string literals and
// punctuation, dropping comments. It reads enough of the language to find
// steps and blocks; it is not a Groovy parser.
func groovyTokens(src string) []groovyToken {
	var tokens []groovyToken
	for i := 0; i < len(src); {
		c := src[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == ';':
			i++
		case strings.HasPrefix(src[i:], "//"):
			i += lineEnd(src[i:])
		case strings.HasPrefix(src[i:], "/*"):
			end := strings.Index(src[i+2:], "*/")
			if end < 0 {
				return tokens
			}
			i += end + 4
		case c == '\'' || c == '"':
			text, n := readGroovyString(src[i:])
			tokens = append(tokens, groovyToken{groovyString, text})
			i += n
		case isIdentStart(c):
			n := 1
			for i+n < len(src) && isIdentPart(src[i+n]) {
				n++
			}
			tokens = append(tokens, groovyToken{groovyIdent, src[i : i+n]})
			i += n
		case strings.ContainsRune("(){}[],:.", rune(c)):
			tokens = append(tokens, groovyToken{groovyPunct, string(c)})
			i++
		default:
			n := 1
			for i+n < len(src) && !slices.Contains([]byte(" \t\r\n;(){}[],:'\""), src[i+n]) && !isIdentStart(src[i+n]) {
				n++
			}
			tokens = append(tokens, groovyToken{groovyOther, src[i : i+n]})
			i += n
		}
	}
	return tokens
}

// readGroovyString reads a quoted string in any of Groovy's four quotings and
// returns its contents and the length consumed. Interpolation is kept as
// written.
func readGroovyString(src string) (string, int) {
	quote := src[:1]
	if strings.HasPrefix(src, strings.Repeat(quote, 3)) {
		quote = strings.Repeat(quote, 3)
	}
	var text strings.Builder
	for i := len(quote); i < len(src); i++ {
		switch {
		case src[i] == '\\' && i+1 < len(src):
			text.WriteByte(src[i+1])
			i++
		case strings.HasPrefix(src[i:], quote):
			return text.String(), i + len(quote)
		case len(quote) == 1 && src[i] == '\n':
			return text.String(), i
		default:
			text.WriteByte(src[i])
		}
	}
	return text.String(), len(src)
}

func lineEnd(s string) int {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return i
	}
	return len(s)
}

func isIdentStart(c byte) bool {
	return c == '_' || c == '$' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

func isIdentPart(c byte) bool {
	return isIdentStart(c) || c >= '0' && c <= '9'
}

// NewJenkinsParser returns a Parser for Jenkins.
func NewJenkinsParser() Parser {
	return &JenkinsParser{}
}
