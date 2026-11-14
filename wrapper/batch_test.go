package wrapper_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

const dispatcher = `:: Build helper
@echo off
SetLocal EnableDelayedExpansion
set GOBIN=%USERPROFILE%\go\bin
set GOTEST=go test -timeout=10m
set GOLINT=%GOBIN%\golangci-lint
if NOT DEFINED TAGS (
    set TAGS=""
)
set PKGS=
for /f %%p in ('go list .\...') do call set "PKGS=%%PKGS%% %%p"

if "%~1"=="build" goto build
if "%~1"=="test" goto test
if /i "%~1" == "LINT" goto lint

:build
go build -tags %TAGS% .\cmd\app
if errorlevel 1 goto fail
go vet .\...
goto :EOF

:test
%GOTEST% -tags %TAGS% ./...%PKGS%
if errorlevel 1 goto fail
goto :EOF

:lint
rem lint everything
%GOLINT% run --timeout 10m
if errorlevel 1 goto fail
goto :EOF

:fail
echo Failed with error #%errorlevel%.
exit /b %errorlevel%
`

func batches() map[string]string {
	return map[string]string{
		"make.bat":          dispatcher,
		"loose.cmd":         "@echo off\r\ngolangci-lint run\r\ngo test ./...\r\necho done\r\n",
		"checked.bat":       "golangci-lint run || exit /b 1\r\nruff check . || echo ignored\r\ngo vet ./... || goto :fail\r\nmypy . || exit /b 0\r\necho done\r\nexit /b 0\r\n",
		"levels.bat":        "semgrep scan\nif %errorlevel% neq 0 exit /b %errorlevel%\ntrivy fs .\nif errorlevel 1 (\n  echo trivy failed\n  exit /b 1\n) else (\n  echo trivy passed\n)\ngitleaks detect\nif !errorlevel! equ 0 echo clean\nexit /b\n",
		"sub.bat":           "call :lint\r\ncall :missing\r\ngo test ./...\r\ngoto :eof\r\n:lint\r\nruff check .\r\nexit /b\r\n",
		"goto.bat":          "if \"%1\"==\"\" goto usage\ngoto %1\n:usage\ntype usage.txt\nexit /b 1\n:lint\nshellcheck x.sh\n",
		"blocks.bat":        "if \"%1\"==\"lint\" (\n  call :lint\n  goto :eof\n) else (\n  go test ./...\n)\nif not exist out (\n  mkdir out\n  exit /b 1\n)\nif \"%1\"==\"x\" (\n  semgrep scan\n) else (\n  trivy fs .\n)\ngo build\ngoto :eof\n:lint\nruff check .\n",
		"nested.bat":        "cd tools\ncall build.cmd lint\npushd ..\ncall scripts\\lint.bat\nmake test\npopd\nmake test\n",
		"tools/build.cmd":   "goto %~1\n:lint\nnpx eslint .\n",
		"scripts/lint.bat":  "yamllint .\n",
		"Makefile":          "test:\n\tgo test -race ./...\n",
		"handoff.bat":       "echo first\nscripts\\lint.bat\necho never\n",
		"loop.bat":          ":again\nruff check .\ngoto again\n",
		"self.bat":          "call self.bat\n",
		"binary.bat":        "golangci-lint run\x00\n",
		"vars.bat":          "set \"FLAGS=--timeout 5m\"\nset /a N=1\nset /p ANSWER=Continue?\nset =x\nset QUOTED=\"a b\"\ngolangci-lint run %FLAGS% %QUOTED% %* %~2 %3 %0\n",
		"continued.bat":     "golangci-lint run ^\n  --timeout 5m\n",
		"unknown-if.bat":    "if exist x goto skip\nif \"%CI%\"==\"true\" exit /b 0\nif \"a\" lss \"b\" goto skip\nif\nif \"a\"==\"b\" ruff check .\nif \"a\"==\"a\" mypy .\nif defined CI semgrep scan\nfor %%f in (*.yml) do yamllint %%f\nfor /d %%d in (*) do (\n  shellcheck %%d\n)\ngo vet\n:skip\n",
		"cd.bat":            "cd\ncd /d %~dp0\ncd ..\npopd\ncall\nmake test\ngoto\n",
		"empty-label.bat":   ":\n:   \ngoto :\n",
		"orphan-close.bat":  ")\n(\ngo vet\n)\n",
		"else-skipped.bat":  "if \"%1\"==\"a\" (\n  if \"%1\"==\"b\" (\n    lint b\n  ) else (\n    lint not b\n  )\n) else (\n  lint not a\n  lint still not a\n)\nif \"%1\"==\"b\" (\n  if \"%1\"==\"a\" (\n    lint a\n  )\n  lint inner\n)\nif \"%1\"==\"b\" (\n  lint unclosed\n",
		"success.bat":       "java -version >NUL 2>&1\nif %ERRORLEVEL% equ 0 goto run\necho no java\nexit /b 1\n:run\nmvn verify\nif not errorlevel 1 (\n  echo passed\n) else (\n  echo failed\n)\nnpm test\nif %errorlevel% lss 1 goto :eof\nexit /b 1\n",
		"chained.bat":       "endlocal & golangci-lint run 2>&1 & call :tests\nset MODE=fast&goto done\n:tests\ngo test ./... && echo ok\nexit /b\n:done\nlint ^& %MODE% & goto :eof\nlint never\n",
		"empty-if.bat":      "if \"a\"==\"a\"\ngo vet\nif errorlevel 1\nif exist y (\n  ruff check .\n) else (\n  mypy .\n)\n",
		"gradlew.bat":       "java -jar gradle-wrapper.jar %*\n",
		"echoes.bat":        "echo Don't run `golangci-lint run` by hand\necho y | sdkmanager --licenses\n1>&2 echo.\ngo vet\necho done\n",
		"exit-in-block.bat": "for %%x in (a) do (\n  exit /b 1\n)\ngo vet\n",
	}
}

func TestBatchFilesFollowTheLabelTheirArgumentSelects(t *testing.T) {
	files := batches()
	lint := []string{"%USERPROFILE%/go/bin/golangci-lint run --timeout 10m"}
	assert.Equal(t, lint, expand(t, files, "./make.bat lint"))
	assert.Equal(t, lint, expand(t, files, `.\make.bat lint`))
	assert.Equal(t, lint, expand(t, files, "make.bat Lint"), "if /i compares without case")
	assert.Equal(t, lint, expand(t, files, "call make.bat lint"))
	assert.Equal(t, lint, expand(t, files, `cmd /c "make.bat lint"`))
	assert.Equal(t, lint, expand(t, files, "cmd.exe /d /c make.bat lint"))
	assert.Equal(t, []string{`go test -timeout=10m -tags "" ./...`}, expand(t, files, "./make.bat test"))
	build := []string{`go build -tags "" ./cmd/app`, "go vet ./..."}
	assert.Equal(t, build, expand(t, files, "./make.bat"), "with no argument the file falls into its first label")
	assert.Equal(t, build, expand(t, files, "./make.bat pkg"))
	assert.Equal(t, []string{"shellcheck x.sh"}, expand(t, files, "goto.bat lint"))
	assert.Equal(t, []string{"type usage.txt || true"}, expand(t, files, "goto.bat"))
}

// An echo's prose is not shell quoting or substitution, so only its status
// and what it pipes into count.
func TestABatchEchoIsNotRead(t *testing.T) {
	assert.Equal(t, []string{"echo y | sdkmanager --licenses || true", "go vet || true"}, expand(t, batches(), "echoes.bat"))
}

// cmd carries on past a failure, so only a checked command or the last one
// decides the file's status.
func TestABatchFileFailsOnlyWhereItChecks(t *testing.T) {
	files := batches()
	assert.Equal(t, []string{"golangci-lint run || true", "go test ./... || true"}, expand(t, files, "loose.cmd"))
	assert.Equal(t, []string{"golangci-lint run", "ruff check . || echo ignored || true", "go vet ./...", "mypy . || exit /b 0 || true"},
		expand(t, files, "checked.bat"))
	assert.Equal(t, []string{"semgrep scan", "trivy fs .", "gitleaks detect || true"},
		expand(t, files, "levels.bat"))
	assert.Equal(t, []string{"ruff check . || true", "go test ./..."}, expand(t, files, "sub.bat"))
}

func TestBatchBlocksAndBranches(t *testing.T) {
	files := batches()
	assert.Equal(t, []string{"ruff check ."}, expand(t, files, "blocks.bat lint"))
	assert.Equal(t, []string{"go test ./... || true", "mkdir out || true", "trivy fs . || true", "go build"}, expand(t, files, "blocks.bat"),
		"an unknown branch runs but cannot exit")
	assert.Equal(t, []string{"mypy . || true", "semgrep scan || true", "yamllint %%f || true", "shellcheck %%d || true", "go vet"},
		expand(t, files, "unknown-if.bat"))
	assert.Equal(t, []string{"lint not b"}, expand(t, files, "else-skipped.bat a"))
	assert.Equal(t, []string{"lint not a || true", "lint still not a || true", "lint inner || true", "lint unclosed"}, expand(t, files, "else-skipped.bat b"))
	assert.Equal(t, []string{"go vet"}, expand(t, files, "exit-in-block.bat"))
	assert.Equal(t, []string{"go vet"}, expand(t, files, "orphan-close.bat"))
}

func TestAnErrorlevelTestFollowsItsSuccessBranch(t *testing.T) {
	assert.Equal(t, []string{"java -version >NUL 2>&1", "mvn verify", "npm test"}, expand(t, batches(), "success.bat"))
}

func TestBatchStatementsChainWithAmpersand(t *testing.T) {
	assert.Equal(t, []string{"golangci-lint run 2>&1 || true", "go test ./... && echo ok || true", "lint ^& fast"}, expand(t, batches(), "chained.bat"))
	assert.Equal(t, []string{"go vet || true", "ruff check . || true", "mypy ."}, expand(t, batches(), "empty-if.bat"))
}

func TestBatchFilesNestAndChangeDirectory(t *testing.T) {
	files := batches()
	assert.Equal(t, []string{"build.cmd lint || true", "npx eslint . || true", "scripts/lint.bat || true", "yamllint . || true", "make test || true", "go test -race ./... || true", "make test"},
		expand(t, files, "nested.bat"))
	assert.Equal(t, []string{"scripts/lint.bat", "yamllint ."}, expand(t, files, "handoff.bat"),
		"running another batch file without call never returns")
	assert.Equal(t, []string{"make test || true", "go test -race ./... || true"}, expand(t, files, "cd.bat"), "a goto to no label ends the file in error")
	assert.Equal(t, []string{"ruff check . || true", "ruff check ."}, expand(t, files, "loop.bat"), "a label is jumped to once")
	assert.Equal(t, []string{"self.bat"}, expand(t, files, "self.bat"))
}

func TestBatchVariablesAndArguments(t *testing.T) {
	files := batches()
	assert.Equal(t, []string{`golangci-lint run --timeout 5m "a b" one "two words" two words  %0`},
		expand(t, files, `vars.bat one "two words"`), "a missing %3 is empty and %0 is left")
	assert.Equal(t, []string{"golangci-lint run --timeout 5m"}, expand(t, files, "continued.bat"))
}

func TestBatchFilesZanadirCannotRead(t *testing.T) {
	files := batches()
	for _, call := range []string{
		"binary.bat",
		"missing.bat",
		"../outside.bat",
		"%SCRIPTS%/make.bat",
		"empty-label.bat",
		"pnpm.cmd exec eslint",
		"cmd /c",
		"cmd",
		"call",
		"./gradlew.bat build",
	} {
		assert.Empty(t, expand(t, files, call), call)
	}
}
