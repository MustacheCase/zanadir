package wrapper_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

const gomakeMain = `package main

import (
	. "github.com/anchore/go-make"
	"github.com/anchore/go-make/lang"
	"github.com/anchore/go-make/tasks/golint"
	"github.com/anchore/go-make/tasks/gotask"
	"github.com/anchore/go-make/tasks/gotest"
)

const tags = "netgo"

func main() {
	Makefile(
		golint.Tasks(),
		golint.CheckLicensesTask(),
		gotest.Tasks(gotest.Name("unit"), gotest.CoverageThreshold(62)),
		gotest.Tasks(gotest.Name("smoke"), gotest.NoCoverage(), options),
		cfg.golint.Tasks(),
		Task{"positional"},
		Task{Name: dynamicName, Run: func() {
			_ = []string{"not", "a", "command"}
			Run(command)
			func() {}()
		}},
		Task{
			Name:   "integration",
			RunsOn: lang.List("test"),
			Run: func() {
				Run("go test -tags="+tags+" ./test/integration/...", run.Env("A", "b"))
				Run(fmt.Sprintf("echo %s", tags))
			},
		},
		&Task{
			Name:         "drift",
			Aliases:      lang.List("check-drift"),
			RunsOn:       lang.List("static-analysis"),
			Dependencies: Deps("generate"),
			Run:          checkDrift,
			Tasks:        []Task{{Name: "drift:nested", Run: func() { Run("echo nested") }}},
		},
		Task{Name: "generate", Run: t.Generate},
		Task{Name: "default", Dependencies: Deps("static-analysis", "test")},
		gotask.Tasks(),
		other.Thing(),
		helper()(),
	)
}

func checkDrift() { Run((` + "`" + `git diff --exit-code` + "`" + `)) }
`

func gomakeRepo() map[string]string {
	return map[string]string{
		"Makefile":           ".PHONY: *\n%:\n\t@go run -C .make . $@\n",
		".make/go.mod":       "module make\n\nrequire github.com/anchore/go-make v0.9.3\n",
		".make/main.go":      gomakeMain,
		".make/main_test.go": "package main\n\nfunc TestX() { Task{Name: \"lint\", Run: func() { Run(\"eslint .\") }} }\n",
		".make/broken.go":    "package main\nfunc {",
		"Taskfile.yaml":      "tasks:\n  docs: mkdocs build\n",
	}
}

func TestGoMakeFollowsTasks(t *testing.T) {
	files := gomakeRepo()
	assert.Equal(t, []string{
		"go run -C .make . static-analysis",
		"go mod tidy -diff", "golangci-lint run", "bouncer check ./...",
		"git diff --exit-code",
	}, expand(t, files, "make static-analysis"))

	assert.Equal(t, []string{
		"go run -C .make . test",
		"go test -coverprofile cover.out -covermode=atomic -coverpkg=./... ./...",
		"go test ./...",
		"go test -tags= ./test/integration/...",
	}, expand(t, files, "make test"))

	assert.Equal(t, []string{"go run -C .make . check-drift", "git diff --exit-code"}, expand(t, files, "make check-drift"))
	assert.Equal(t, []string{"go run -C .make . check-licenses", "bouncer check ./..."}, expand(t, files, "make check-licenses"))
	assert.Equal(t, []string{"go run -C .make . docs", "mkdocs build"}, expand(t, files, "make docs"), "gotask hands other names to the Taskfile")
	assert.Equal(t, []string{"golangci-lint run"}, expand(t, files, "go run -race -C .make . lint"))
	assert.Contains(t, expand(t, files, "go run -C .make . default"), "go test ./...")
}

func TestGoMakeNeedsAGoMakeModule(t *testing.T) {
	files := gomakeRepo()
	files[".make/go.mod"] = "module make\n"
	assert.Equal(t, []string{"go run -C .make . lint"}, expand(t, files, "make lint"))
	assert.Nil(t, expand(t, gomakeRepo(), "go run -C .make ."), "no task: go-make prints its help")
	assert.Nil(t, expand(t, gomakeRepo(), "go run -C ../outside . lint"))
	assert.Nil(t, expand(t, gomakeRepo(), "go build ./... && go run"))
	assert.Nil(t, expand(t, gomakeRepo(), "go run -C .make . unknown && go run -C .make . unknown"))
}
