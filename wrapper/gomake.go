package wrapper

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/MustacheCase/zanadir/internal/repofs"
)

// A go-make build (github.com/anchore/go-make) hands each make target to
// "go run -C .make . <target>", whose tasks are Go code: Task literals that
// call Run with shell commands, and the library's packaged tasks below.
type gomakeTask struct {
	name     string
	aliases  []string
	deps     []string
	runsOn   []string
	commands []string
}

type gomakeBuild struct {
	dir   string
	tasks []*gomakeTask
	// gotask.Tasks() forwards every other name to the Taskfile.
	gotask bool
}

// What go-make v0.9's packaged tasks run, since only their constructor
// calls are in the repository.
var gomakeLibrary = map[string][]*gomakeTask{
	"golint.Tasks": {
		{name: "static-analysis", runsOn: []string{"default"},
			commands: []string{"go mod tidy -diff", "golangci-lint run", "bouncer check ./..."}},
		{name: "format", commands: []string{"golangci-lint fmt", "go mod tidy"}},
		{name: "lint", commands: []string{"golangci-lint run"}},
		{name: "lint:fix", deps: []string{"format"}, commands: []string{"golangci-lint run --fix"}},
	},
	"golint.StaticAnalysisTask": {{name: "static-analysis", runsOn: []string{"default"},
		commands: []string{"go mod tidy -diff", "golangci-lint run", "bouncer check ./..."}}},
	"golint.LintTask":          {{name: "lint", commands: []string{"golangci-lint run"}}},
	"goreleaser.Tasks":         {{name: "ci:release", aliases: []string{"ci-release"}, commands: []string{"goreleaser release --clean"}}},
	"goreleaser.ReleaseTask":   {{name: "ci:release", aliases: []string{"ci-release"}, commands: []string{"goreleaser release --clean"}}},
	"golint.CheckLicensesTask": {{name: "check-licenses", commands: []string{"bouncer check ./..."}}},
}

func (e *Expander) goRun(dir string, args []string) {
	if len(args) == 0 || args[0] != "run" {
		return
	}
	module, args := dir, args[1:]
	for len(args) > 1 && strings.HasPrefix(args[0], "-") {
		if args[0] == "-C" {
			module, args = filepath.Join(module, args[1]), args[1:]
		}
		args = args[1:]
	}
	if len(args) < 2 {
		return
	}
	module, ok := e.within(module, args[0])
	if !ok {
		return
	}
	build := e.gomake(module)
	if build == nil {
		return
	}
	for _, name := range args[1:] {
		e.gomakeTask(build, dir, name)
	}
}

// gomakeTask runs a task's dependencies, its commands, and then every task
// that runs on it, as go-make does.
func (e *Expander) gomakeTask(b *gomakeBuild, dir, name string) {
	if !e.once("go-make " + b.dir + " " + name) {
		return
	}
	found := name == "test"
	for _, t := range b.tasks {
		if t.name != name && !slices.Contains(t.aliases, name) {
			continue
		}
		found = true
		for _, dep := range t.deps {
			e.gomakeTask(b, dir, dep)
		}
		for _, command := range t.commands {
			e.emit(dir, command)
		}
	}
	for _, t := range b.tasks {
		if slices.Contains(t.runsOn, name) {
			e.gomakeTask(b, dir, t.name)
		}
	}
	if !found && b.gotask {
		e.task(dir, []string{name})
	}
}

// gomake reads the go-make build in module, or returns nil when module is
// not one.
func (e *Expander) gomake(module string) *gomakeBuild {
	if b, ok := e.gomakes[module]; ok {
		return b
	}
	e.gomakes[module] = nil
	mod, err := repofs.ReadFile(e.root, filepath.Join(module, "go.mod"))
	if err != nil || !strings.Contains(string(mod), "github.com/anchore/go-make") {
		return nil
	}

	b := &gomakeBuild{dir: module}
	files, _ := filepath.Glob(filepath.Join(module, "*.go"))
	var parsed []*ast.File
	funcs := map[string]*ast.FuncDecl{}
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		src, err := repofs.ReadFile(e.root, path)
		if err != nil {
			continue
		}
		f, err := parser.ParseFile(token.NewFileSet(), path, src, 0)
		if err != nil {
			continue
		}
		parsed = append(parsed, f)
		for _, decl := range f.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Recv == nil {
				funcs[fn.Name.Name] = fn
			}
		}
	}
	for _, f := range parsed {
		ast.Inspect(f, func(node ast.Node) bool {
			switch n := node.(type) {
			case *ast.CompositeLit:
				if typeName(n.Type) == "Task" {
					b.tasks = append(b.tasks, gomakeLiteral(n, funcs))
				}
			case *ast.CallExpr:
				b.library(n)
			}
			return true
		})
	}
	e.gomakes[module] = b
	return b
}

// library adds the tasks a call to one of go-make's packaged constructors
// defines.
func (b *gomakeBuild) library(call *ast.CallExpr) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return
	}
	pkg, ok := sel.X.(*ast.Ident)
	if !ok {
		return
	}
	switch name := pkg.Name + "." + sel.Sel.Name; name {
	case "gotask.Tasks":
		b.gotask = true
	case "gotest.Tasks":
		test := &gomakeTask{name: "unit", runsOn: []string{"test"},
			commands: []string{"go test -coverprofile cover.out -covermode=atomic -coverpkg=./... ./..."}}
		for _, arg := range call.Args {
			switch option := typeName(callee(arg)); option {
			case "Name":
				test.name = firstString(arg)
			case "NoCoverage":
				test.commands = []string{"go test ./..."}
			}
		}
		b.tasks = append(b.tasks, test)
	default:
		b.tasks = append(b.tasks, gomakeLibrary[name]...)
	}
}

func gomakeLiteral(lit *ast.CompositeLit, funcs map[string]*ast.FuncDecl) *gomakeTask {
	t := &gomakeTask{}
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		switch typeName(kv.Key) {
		case "Name":
			t.name = firstString(kv.Value)
		case "Aliases":
			t.aliases = strings.Fields(strings.Join(allStrings(kv.Value), " "))
		case "Dependencies":
			t.deps = allStrings(kv.Value)
		case "RunsOn":
			t.runsOn = allStrings(kv.Value)
		case "Run":
			var body ast.Node = kv.Value
			if ident, ok := kv.Value.(*ast.Ident); ok && funcs[ident.Name] != nil {
				body = funcs[ident.Name].Body
			}
			t.commands = runCommands(body)
		}
	}
	return t
}

// runCommands collects the commands a task body passes to Run. Only the
// literal parts of a command built by concatenation are kept.
func runCommands(body ast.Node) []string {
	var commands []string
	ast.Inspect(body, func(node ast.Node) bool {
		if _, nested := node.(*ast.CompositeLit); nested {
			return false
		}
		if call, ok := node.(*ast.CallExpr); ok && typeName(call.Fun) == "Run" && len(call.Args) > 0 {
			if command := literal(call.Args[0]); strings.TrimSpace(command) != "" {
				commands = append(commands, command)
			}
		}
		return true
	})
	return commands
}

func literal(expr ast.Expr) string {
	switch x := expr.(type) {
	case *ast.BasicLit:
		s, _ := strconv.Unquote(x.Value)
		return s
	case *ast.BinaryExpr:
		return literal(x.X) + literal(x.Y)
	case *ast.ParenExpr:
		return literal(x.X)
	}
	return ""
}

func firstString(expr ast.Expr) string {
	if all := allStrings(expr); len(all) > 0 {
		return all[0]
	}
	return ""
}

func allStrings(expr ast.Expr) []string {
	var all []string
	ast.Inspect(expr, func(node ast.Node) bool {
		if lit, ok := node.(*ast.BasicLit); ok && lit.Kind == token.STRING {
			all = append(all, literal(lit))
		}
		return true
	})
	return all
}

// typeName is the last name in an identifier or a selector: Task in Task{}
// and gomake.Task{}.
func typeName(expr ast.Expr) string {
	switch x := expr.(type) {
	case *ast.Ident:
		return x.Name
	case *ast.SelectorExpr:
		return x.Sel.Name
	}
	return ""
}

func callee(expr ast.Expr) ast.Expr {
	if call, ok := expr.(*ast.CallExpr); ok {
		return call.Fun
	}
	return nil
}
