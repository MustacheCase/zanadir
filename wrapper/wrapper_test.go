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
		`echo "$HOME" || true`,
	}, expand(t, files, "make -j4 V=1 lint"))
	assert.Equal(t, []string{"go test ./..."}, expand(t, files, "make test"))
	assert.Equal(t, []string{"govulncheck ./..."}, expand(t, files, "make extra"))
	assert.Contains(t, expand(t, files, "make ci"), ".tool/golangci-lint run --timeout 5m -v")
	assert.Equal(t, []string{"go mod tidy -diff", ".tool/golangci-lint run --timeout 5m -v", `echo "$HOME" || true`},
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
      - [not, a, command]
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
	assert.Equal(t, []string{"eslint ."}, expand(t, files, "task --silent -d web lint CI=true -- --fix"))
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
	assert.Equal(t, []string{"ruff check . || true"}, expand(t, files, "python -m tox -e lint,lint -- -x && tox -e lint"))
	assert.Equal(t, []string{"uv sync", "pytest --cov src {posargs}", "ruff check . || true"}, expand(t, files, "tox"))
	assert.Equal(t, []string{"uv sync", "pytest --cov src {posargs}"},
		expand(t, map[string]string{"tox.ini": "[testenv]\ncommands =\n    uv sync\n    pytest --cov src {posargs}\n"}, "poetry run tox"))
	assert.Nil(t, expand(t, map[string]string{}, "tox"))
}

func TestNothingToFollow(t *testing.T) {
	assert.Nil(t, expand(t, map[string]string{"Makefile": "lint:\n\techo\n"}, "golangci-lint run\n\nsudo FOO=1"))
}

// A wrapper's own way of ignoring a failure is kept as "|| true", which
// grading reads as it reads the shell's.
func TestWrappersKeepTheirIgnoreMarkers(t *testing.T) {
	makefile := map[string]string{"Makefile": "lint:\n\t-golangci-lint run\n\tgo vet ./...\nall:\n\t-$(MAKE) lint\ntest:\n\tgo test ./...\n"}
	assert.Equal(t, []string{"golangci-lint run || true", "go vet ./..."}, expand(t, makefile, "make lint"))
	assert.Equal(t, []string{"make lint || true", "golangci-lint run || true", "go vet ./... || true"}, expand(t, makefile, "make all"), "ignoring reaches through a nested make")
	for _, flags := range []string{"-i", "-ik", "--ignore-errors"} {
		assert.Equal(t, []string{"go test ./... || true"}, expand(t, makefile, "make "+flags+" test"), flags)
	}
	assert.Equal(t, []string{"go test ./..."}, expand(t, makefile, "make -k test"), "-k keeps going but still fails")

	ignoring := map[string]string{"Makefile": ".IGNORE: lint\nlint:\n\tgolangci-lint run\ntest:\n\tgo test ./...\n"}
	assert.Equal(t, []string{"golangci-lint run || true"}, expand(t, ignoring, "make lint"))
	assert.Equal(t, []string{"go test ./..."}, expand(t, ignoring, "make test"))
	assert.Equal(t, []string{"go test ./... || true"}, expand(t, map[string]string{"Makefile": ".IGNORE:\ntest:\n\tgo test ./...\n"}, "make test"))

	tox := map[string]string{"tox.ini": "[testenv]\nignore_outcome = True\ncommands = pytest\n[testenv:lint]\nignore_errors = true\ncommands = ruff check .\n"}
	assert.Equal(t, []string{"pytest || true"}, expand(t, tox, "tox -e py"))
	assert.Equal(t, []string{"ruff check . || true"}, expand(t, tox, "tox -e lint"), "an env inherits [testenv]'s ignore_outcome")
	assert.Equal(t, []string{"ruff check ."}, expand(t, map[string]string{"tox.ini": "[testenv:lint]\nignore_errors = true\ncommands = ruff check .\n"}, "tox -e lint"),
		"ignore_errors still fails the environment")

	task := map[string]string{"Taskfile.yml": "tasks:\n  secrets:\n    cmds:\n      - cmd: gitleaks detect\n        ignore_error: true\n      - trufflehog git .\n  all:\n    ignore_error: true\n    cmds: [semgrep scan, {task: secrets}]\n"}
	assert.Equal(t, []string{"gitleaks detect || true", "trufflehog git ."}, expand(t, task, "task secrets"))
	assert.Equal(t, []string{"semgrep scan || true", "gitleaks detect || true", "trufflehog git . || true"}, expand(t, task, "task all"))
}
