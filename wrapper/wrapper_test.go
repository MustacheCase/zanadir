package wrapper_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/MustacheCase/zanadir/internal/testrepo"
	"github.com/MustacheCase/zanadir/wrapper"
	"github.com/stretchr/testify/assert"
)

func expand(t *testing.T, files map[string]string, script string) []string {
	t.Helper()
	out := wrapper.New(testrepo.Write(t, files)).Expand(script)
	if out == "" {
		return nil
	}
	return strings.Split(out, "\n")
}

func TestMakeFollowsTargets(t *testing.T) {
	makefile := `TOOLS := .tool
LINT ?= $(TOOLS)/golangci-lint
LINT ?= ignored
FLAGS = --timeout 5m
FLAGS += -v
VERSION != git describe

.PHONY: lint test
define HELP
	echo not a recipe
endef

lint: tidy ## run the linters
	@$(LINT) run $(FLAGS) $(shell nproc)
	-echo "$$HOME"

tidy:
	go mod tidy -diff

test: unit
check: VERBOSE = 1
ci: ; $(MAKE) lint
%.o: %.c
	cc -c $<
ifeq ($(CI),true)
unit:
	go test ./...
endif
include mk/*.mk
`
	files := map[string]string{"Makefile": makefile, "mk/extra.mk": "extra:\n\tgovulncheck ./...\n"}

	assert.Equal(t, []string{
		"go mod tidy -diff",
		".tool/golangci-lint run --timeout 5m -v",
		`echo "$HOME"`,
	}, expand(t, files, "make -j4 V=1 lint"))
	assert.Equal(t, []string{"go test ./..."}, expand(t, files, "make test"))
	assert.Equal(t, []string{"govulncheck ./..."}, expand(t, files, "make extra"))
	assert.Contains(t, expand(t, files, "make ci"), ".tool/golangci-lint run --timeout 5m -v")
	assert.Equal(t, []string{"go mod tidy -diff", ".tool/golangci-lint run --timeout 5m -v", `echo "$HOME"`},
		expand(t, files, "make"), "the first target is the default goal")
	assert.Nil(t, expand(t, files, "make check"), "a target-specific variable is not a rule")
	assert.Nil(t, expand(t, files, "make unknown"))
}

func TestMakeCatchAllHandsTheGoalOn(t *testing.T) {
	files := map[string]string{
		"Makefile": `TASK = .tool/task
.DEFAULT_GOAL := default
$(TASK):
	curl -sSfL https://example.com/install.sh | sh
%:
	@make --silent $(TASK)
	@$(TASK) $@
`,
		"Taskfile.yml": "tasks:\n  lint: ruff check .\n  default: [pytest]\n",
	}
	assert.Equal(t, []string{
		"make --silent .tool/task",
		"curl -sSfL https://example.com/install.sh | sh",
		".tool/task lint",
		"ruff check .",
	}, expand(t, files, "make lint"))
	assert.Contains(t, expand(t, files, "make"), "pytest")
}

func TestMakeFindsTheMakefile(t *testing.T) {
	files := map[string]string{
		"sub/GNUmakefile": "lint:\n\tgolangci-lint run\n",
		"ci.mk":           "lint:\n\teslint .\n",
	}
	assert.Equal(t, []string{"golangci-lint run"}, expand(t, files, "make -C sub lint"))
	assert.Equal(t, []string{"golangci-lint run"}, expand(t, files, "cd sub && make lint"))
	assert.Equal(t, []string{"eslint ."}, expand(t, files, "make -f ci.mk lint"))
	assert.Nil(t, expand(t, files, "make lint"), "no makefile in the root")
	assert.Nil(t, expand(t, files, "make -C ../elsewhere lint"))
	assert.Nil(t, expand(t, files, "cd /etc && make lint"))
}

func TestMakeStopsOnCyclesAndSelfIncludes(t *testing.T) {
	files := map[string]string{"Makefile": "include Makefile\na: b\n\techo a\nb: a\n\techo b\n"}
	assert.Equal(t, []string{"echo b", "echo a"}, expand(t, files, "make a"))
}

func TestFollowingIsBounded(t *testing.T) {
	var chain strings.Builder
	for i := range 20 {
		fmt.Fprintf(&chain, "t%d:\n\techo %d\n\tmake t%d\n", i, i, i+1)
	}
	lines := expand(t, map[string]string{"Makefile": chain.String()}, "make t0")
	assert.NotContains(t, lines, "echo 19")
	assert.Contains(t, lines, "echo 0")

	huge := "all:\n" + strings.Repeat("\techo step\n", 3000)
	assert.Len(t, expand(t, map[string]string{"Makefile": huge}, "make"), 2000)
}

func TestTaskFollowsTasks(t *testing.T) {
	files := map[string]string{
		"Taskfile.yaml": `version: "3"
tasks:
  default:
    cmds:
      - task: static-analysis
      - cmd: go test -cover ./...
  static-analysis:
    deps: [tools, {task: generate}]
    cmd: golangci-lint run
    cmds:
      - "bouncer check ./..."
      - for: [a, b]
        cmd: echo {{.ITEM}}
  tools: binny install
  generate:
    - go generate ./...
  broken:
    cmd: [not, a, string]
`,
		"web/Taskfile.yml": "tasks:\n  lint: eslint .\n",
	}
	assert.Equal(t, []string{
		"binny install",
		"go generate ./...",
		"golangci-lint run",
		"bouncer check ./...",
		"echo {{.ITEM}}",
		"go test -cover ./...",
	}, expand(t, files, "task"))
	assert.Equal(t, []string{"eslint ."}, expand(t, files, "task -d web lint -- --fix"))
	assert.Equal(t, []string{"eslint ."}, expand(t, files, "task --taskfile web/Taskfile.yml lint"))
	assert.Equal(t, []string{"binny install", "go generate ./..."}, expand(t, files, "task tools; task generate"))
	assert.Nil(t, expand(t, files, "task broken missing"))
	assert.Nil(t, expand(t, map[string]string{"Taskfile.yml": "tasks: [oops"}, "task lint"))
	assert.Nil(t, expand(t, map[string]string{}, "task lint"))
}

func TestToxRunsEnvironments(t *testing.T) {
	files := map[string]string{"tox.ini": `# comment
[tox]
envlist = py313, lint

[testenv]
deps = pytest
commands =
    uv sync
    pytest --cov src {posargs}

[testenv:lint]
commands = - ruff check .
not a key
`}
	assert.Equal(t, []string{"uv sync", "pytest --cov src {posargs}"}, expand(t, files, `uv run --frozen tox -e "$TOXENV"`))
	assert.Equal(t, []string{"ruff check ."}, expand(t, files, "python -m tox -e lint,lint -- -x && tox -e lint"))
	assert.Equal(t, []string{"uv sync", "pytest --cov src {posargs}", "ruff check ."}, expand(t, files, "tox"))
	assert.Equal(t, []string{"uv sync", "pytest --cov src {posargs}"},
		expand(t, map[string]string{"tox.ini": "[testenv]\ncommands =\n    uv sync\n    pytest --cov src {posargs}\n"}, "poetry run tox"))
	assert.Nil(t, expand(t, map[string]string{}, "tox"))
}

func TestNothingToFollow(t *testing.T) {
	assert.Nil(t, expand(t, map[string]string{"Makefile": "lint:\n\techo\n"}, "golangci-lint run\n\nsudo FOO=1"))
}
