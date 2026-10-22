package shell_test

import (
	"strings"
	"testing"

	"github.com/MustacheCase/zanadir/internal/shell"
	"github.com/stretchr/testify/assert"
)

func TestCommands(t *testing.T) {
	assert.Equal(t, []string{"set -e", "make lint", "true", "cat a", "grype", "go test  \t ./...", "echo done"},
		shell.Commands("set -e\n# gitleaks is disabled for now\nmake lint || true; cat a | grype\n\ngo test \\\n\t ./... && echo done"))
	assert.Nil(t, shell.Commands(" \n "))
}

func TestWords(t *testing.T) {
	assert.Equal(t, []string{"make", "lint"}, shell.Words("sudo CI=1 GOFLAGS=-mod=mod make lint"))
	assert.Empty(t, shell.Words("FOO=bar"))
}

func TestSwallowed(t *testing.T) {
	runs := func(command string) bool { return strings.Contains(command, "gitleaks") }
	for script, want := range map[string]bool{
		"gitleaks detect || true":                               true,
		"gitleaks detect || echo 'leaks found'":                 true,
		"if ! gitleaks detect; then echo found; fi":             true,
		"if gitleaks detect; then echo clean; fi":               true,
		"! gitleaks detect":                                     true,
		"while ! gitleaks detect; do sleep 1; done":             true,
		"set -e\ngitleaks detect --report r.json \\\n  || true": true,
		"gitleaks detect":                                       false,
		"gitleaks detect || exit 1":                             false,
		"gitleaks detect || exit $?":                            false,
		"gitleaks detect || { echo 'leaks found'; exit 1; }":    false,
		"gitleaks detect || false":                              false,
		"gitleaks detect || true\ngitleaks detect --staged":     false,
		"make || gitleaks detect":                               false,
		`if [ "$TOOL" = gitleaks ]; then make; fi`:              false,
		"pip install gitleaks || true":                          false,
		"echo gitleaks || true":                                 false,
		"# gitleaks detect || true":                             false,
		"":                                                      false,
	} {
		assert.Equal(t, want, shell.Swallowed(script, runs), script)
	}
}

func TestPassive(t *testing.T) {
	for command, want := range map[string]bool{
		"chmod +x bin/grype":           true,
		"python -m pip install bandit": true,
		"uv tool install ruff":         true,
		"trivy --version":              true,
		"npm run lint":                 false,
		"trivy fs --version-check .":   false,
		"go run ./cmd/gosec ./...":     false,
	} {
		assert.Equal(t, want, shell.Passive(strings.Fields(command)), command)
	}
}

func TestCommandsRespectQuotesSubstitutionsCommentsAndHeredocs(t *testing.T) {
	assert.Equal(t, []string{`addlicense -c "A; B | C" .`, "echo $(date | tr a b)", "make build", "run `a | b`", "cat <<EOF", "make"},
		shell.Commands("addlicense -c \"A; B | C\" .\necho $(date | tr a b)\nmake build  # TODO: add gitleaks\nrun `a | b`\ncat <<EOF\n- id: gitleaks\nEOF\nmake"))
	assert.Equal(t, []string{`cat <<-'END' > f`, "ok"}, shell.Commands("cat <<-'END' > f\n  semgrep scan\n  END\nok"))
	assert.Equal(t, []string{"cat <<EOF"}, shell.Commands("cat <<EOF\nnever closed"))
	assert.Equal(t, []string{`echo 'it''s'`, `a\;b`, "x &"}, shell.Commands(`echo 'it''s'; a\;b; x &`))
	assert.Equal(t, []string{"echo ${#arr}", "curl https://x/#frag"}, shell.Commands("echo ${#arr}\ncurl https://x/#frag"))
}

func TestWordsKeepQuotesAndSubstitutionsWhole(t *testing.T) {
	assert.Equal(t, []string{"make", "lint"}, shell.Words("NODE=$(command -v node) make lint"))
	assert.Equal(t, []string{"git", "commit", "-m", `"bump golangci-lint"`}, shell.Words(`git commit -m "bump golangci-lint"`))
	assert.Equal(t, []string{"echo", "a"}, shell.Words("echo a # trailing"))
}

func TestInvoked(t *testing.T) {
	for command, want := range map[string]string{
		`addlicense -c "OpenSSF Scorecard Authors" .`:            "addlicense -c .",
		`git commit -m "chore: bump golangci-lint"`:              "git commit -m",
		"ct lint-and-install --charts helm/trivy":                "ct lint-and-install --charts",
		"cargo build --bin ruff":                                 "cargo build --bin",
		"docker build -t ghcr.io/aquasecurity/trivy:dev .":       "docker build -t .",
		`bash -c "gitleaks detect --no-git"`:                     `bash -c "gitleaks detect --no-git"`,
		"docker run --rm -v \"$PWD:/src\" aquasec/trivy fs /src": "docker run --rm -v aquasec/trivy fs /src",
		"python -m pyright src/requests/":                        "python -m pyright src/requests/",
		"npx eslint .":                                           "npx eslint .",
		"mvn org.owasp:dependency-check-maven:check":             "mvn org.owasp:dependency-check-maven:check",
	} {
		assert.Equal(t, want, strings.Join(shell.Invoked(shell.Words(command)), " "), command)
	}
	assert.Nil(t, shell.Invoked(nil))
}

func TestATestOnlyComparesText(t *testing.T) {
	for _, command := range []string{`if [ "$PR_AUTHOR" != "trivy-bot" ]`, `[[ -f trivy.yaml ]]`, "test -x bin/grype", "!", "while"} {
		assert.True(t, shell.Passive(shell.Words(command)), command)
	}
	assert.False(t, shell.Passive(shell.Words("if gitleaks detect")))
}

func TestSwallowedCommand(t *testing.T) {
	assert.True(t, shell.SwallowedCommand("make lint || true", "make lint"))
	assert.True(t, shell.SwallowedCommand("if make lint; then echo ok; fi", "if make lint"))
	assert.False(t, shell.SwallowedCommand("make lint || exit 1", "make lint"))
	assert.False(t, shell.SwallowedCommand("make lint\nmake lint || true", "make lint"))
	assert.False(t, shell.SwallowedCommand("make test || true", "make lint"))
}
