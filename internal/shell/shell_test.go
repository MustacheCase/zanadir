package shell_test

import (
	"strings"
	"testing"
	"time"

	"github.com/MustacheCase/zanadir/internal/shell"
	"github.com/stretchr/testify/assert"
)

func TestCommands(t *testing.T) {
	assert.Equal(t, []string{"set -e", "make lint", "true", "cat a", "grype", "go test  \t ./...", "echo done"},
		shell.Commands("set -e\n# gitleaks is disabled for now\nmake lint || true; cat a | grype\n\ngo test \\\n\t ./... && echo done"))
	assert.Nil(t, shell.Commands(" \n "))
}

// A bash backtick that opens a substitution has no space before it.
func TestCommandsJoinPowerShellContinuations(t *testing.T) {
	assert.Equal(t, []string{"Invoke-Pester   -Path tests   -CI", "echo done"},
		shell.Commands("Invoke-Pester `\n  -Path tests `\n  -CI\necho done"))
	assert.Equal(t, []string{"Invoke-Pester   -CI"}, shell.Commands("Invoke-Pester `\r\n  -CI"))
	assert.Equal(t, []string{"now=`\ndate`"}, shell.Commands("now=`\ndate`"))
	assert.Equal(t, []string{"Invoke-ScriptAnalyzer -Path .   -EnableExit"}, shell.Lines("Invoke-ScriptAnalyzer -Path . `\n  -EnableExit"))
}

func TestWords(t *testing.T) {
	assert.Equal(t, []string{"make", "lint"}, shell.Words("sudo CI=1 GOFLAGS=-mod=mod make lint"))
	assert.Empty(t, shell.Words("FOO=bar"))
	for command, want := range map[string][]string{
		"timeout 10m make lint":                   {"make", "lint"},
		"sudo -E make lint":                       {"make", "lint"},
		"sudo -u runner make lint":                {"make", "lint"},
		"sudo -E timeout 10m nice -n 5 make lint": {"make", "lint"},
		"env -i A=1 B=2 make lint":                {"make", "lint"},
		"taskset -c 5-7 k6 run":                   {"k6", "run"},
		"nice make lint":                          {"make", "lint"},
		"stdbuf -oL make lint":                    {"make", "lint"},
		"/usr/bin/time -v make lint":              {"make", "lint"},
		"exec -a lint make lint":                  {"make", "lint"},
		"command make lint":                       {"make", "lint"},
		"xvfb-run -a npm test":                    {"npm", "test"},
		"timeout 10m":                             {},
		"env":                                     {},
		"env FOO=1":                               {},
		"sudo":                                    {},
		"command -v k6":                           {},
		"taskset -p 0x3 1234":                     {},
		"sudo -b k6 run x":                        {},
		"timeout --frobnicate 5 k6 run":           {"timeout", "--frobnicate", "5", "k6", "run"},
		"nohup k6 run x":                          {"nohup", "k6", "run", "x"},
		"if timeout 10m make lint":                {"if", "timeout", "10m", "make", "lint"},
	} {
		assert.Equal(t, want, shell.Words(command), command)
	}
}

func TestNormalizeCommand(t *testing.T) {
	for command, want := range map[string]string{
		`"/tmp/my grant" check .`:                       `"/tmp/my grant" check .`,
		`"/tmp/grant/" check .`:                         `"/tmp/grant/" check .`,
		`"$RUNNER_TEMP/grant" check dir:.`:              `grant check dir:.`,
		`'/opt/my tools/grant' list`:                    `grant list`,
		`./bin/k6 run load.js`:                          `k6 run load.js`,
		`if ! sudo CI=1 "/opt/grant" check .`:           `if ! sudo CI=1 grant check .`,
		`TRIVY_EXIT_CODE=1 "./bin/trivy" fs "some dir"`: `TRIVY_EXIT_CODE=1 trivy fs "some dir"`,
		`git commit -m "bump grant"`:                    `git commit -m "bump grant"`,
		`"$TOOL" check .`:                               `"$TOOL" check .`,
		`"/bin/grant check .`:                           `"/bin/grant check .`,
		`"" check`:                                      `"" check`,
		``:                                              ``,
		// Wrappers stay as written; the program behind them is normalised.
		`taskset -c 5-7 "$root/k6" run "$script"`:                                     `taskset -c 5-7 k6 run "$script"`,
		`sudo -E "./bin/trivy" fs .`:                                                  `sudo -E trivy fs .`,
		`sudo -E -H "./bin/trivy" fs .`:                                               `sudo -E -H trivy fs .`,
		`sudo -u runner "./bin/trivy" fs .`:                                           `sudo -u runner trivy fs .`,
		`sudo -uroot "./bin/trivy" fs .`:                                              `sudo -uroot trivy fs .`,
		`sudo --user=runner "./bin/trivy" fs .`:                                       `sudo --user=runner trivy fs .`,
		`sudo --preserve-env=PATH "./bin/trivy" fs .`:                                 `sudo --preserve-env=PATH trivy fs .`,
		`sudo -u runner -g docker "./bin/trivy" fs .`:                                 `sudo -u runner -g docker trivy fs .`,
		`sudo -- "./bin/trivy" fs .`:                                                  `sudo -- trivy fs .`,
		`sudo CI=1 "./bin/trivy" fs .`:                                                `sudo CI=1 trivy fs .`,
		`env -i PATH="$PATH" "./bin/k6" run t.js`:                                     `env -i PATH="$PATH" k6 run t.js`,
		`env - PATH="$PATH" "./bin/k6" run t.js`:                                      `env - PATH="$PATH" k6 run t.js`,
		`env -u HOME "./bin/k6" run t.js`:                                             `env -u HOME k6 run t.js`,
		`env --unset=HOME "./bin/k6" run t.js`:                                        `env --unset=HOME k6 run t.js`,
		`env -C /src "./bin/k6" run t.js`:                                             `env -C /src k6 run t.js`,
		`env -- "./bin/k6" run t.js`:                                                  `env -- k6 run t.js`,
		`timeout 10m "./bin/k6" run t.js`:                                             `timeout 10m k6 run t.js`,
		`timeout 600 "./bin/k6" run t.js`:                                             `timeout 600 k6 run t.js`,
		`timeout --signal=KILL 5m "./bin/k6" run`:                                     `timeout --signal=KILL 5m k6 run`,
		`timeout -s KILL -k 30s 5m "./bin/k6" run`:                                    `timeout -s KILL -k 30s 5m k6 run`,
		`timeout -s 9 5m "./bin/k6" run`:                                              `timeout -s 9 5m k6 run`,
		`timeout --preserve-status 5m "./bin/k6" run`:                                 `timeout --preserve-status 5m k6 run`,
		`taskset 0x3 "./bin/k6" run t.js`:                                             `taskset 0x3 k6 run t.js`,
		`taskset -a -c 0-3 "./bin/k6" run`:                                            `taskset -a -c 0-3 k6 run`,
		`taskset --cpu-list 0-3 "./bin/k6" run`:                                       `taskset --cpu-list 0-3 k6 run`,
		`nice "./bin/trivy" fs .`:                                                     `nice trivy fs .`,
		`nice -n 10 "./bin/trivy" fs .`:                                               `nice -n 10 trivy fs .`,
		`nice -n -5 "./bin/trivy" fs .`:                                               `nice -n -5 trivy fs .`,
		`nice -n10 "./bin/trivy" fs .`:                                                `nice -n10 trivy fs .`,
		`nice --adjustment=10 "./bin/trivy" fs .`:                                     `nice --adjustment=10 trivy fs .`,
		`nice -10 "./bin/trivy" fs .`:                                                 `nice -10 trivy fs .`,
		`nice --10 "./bin/trivy" fs .`:                                                `nice --10 trivy fs .`,
		`stdbuf -oL "./bin/trivy" fs .`:                                               `stdbuf -oL trivy fs .`,
		`stdbuf -o L -e L "./bin/trivy" fs .`:                                         `stdbuf -o L -e L trivy fs .`,
		`stdbuf --output=L "./bin/trivy" fs .`:                                        `stdbuf --output=L trivy fs .`,
		`time "./bin/trivy" fs .`:                                                     `time trivy fs .`,
		`time -p "./bin/trivy" fs .`:                                                  `time -p trivy fs .`,
		`/usr/bin/time -v "./bin/trivy" fs .`:                                         `/usr/bin/time -v trivy fs .`,
		`/usr/bin/time -f "%e %M" "./bin/trivy" fs .`:                                 `/usr/bin/time -f "%e %M" trivy fs .`,
		`"/usr/bin/timeout" 10m "./bin/k6" run`:                                       `"/usr/bin/timeout" 10m k6 run`,
		`command "./bin/trivy" fs .`:                                                  `command trivy fs .`,
		`command -p "./bin/trivy" fs .`:                                               `command -p trivy fs .`,
		`exec "./bin/trivy" fs .`:                                                     `exec trivy fs .`,
		`exec -a scan "./bin/trivy" fs .`:                                             `exec -a scan trivy fs .`,
		`xvfb-run -a "./node_modules/.bin/cypress" run`:                               `xvfb-run -a cypress run`,
		`xvfb-run -s "-screen 0 1x1x24" "./node_modules/.bin/cypress" run`:            `xvfb-run -s "-screen 0 1x1x24" cypress run`,
		`xvfb-run --server-args="-screen 0 1x1x24" "./node_modules/.bin/cypress" run`: `xvfb-run --server-args="-screen 0 1x1x24" cypress run`,
		`sudo -E timeout 10m "./k6" run`:                                              `sudo -E timeout 10m k6 run`,
		`timeout 10m sudo -u runner nice -n 5 "./k6" run`:                             `timeout 10m sudo -u runner nice -n 5 k6 run`,
		`if ! timeout 10m "./bin/k6" run t.js`:                                        `if ! timeout 10m k6 run t.js`,
		// A wrapper's own arguments never become the program.
		`taskset -c 5-7`: `taskset -c 5-7`,
		`timeout 10m`:    `timeout 10m`,
		`sudo -u runner`: `sudo -u runner`,
		`env`:            `env`,
		`sudo`:           `sudo`,
		// Nothing runs that the step waits for.
		`command -v "./bin/k6"`:      `command -v "./bin/k6"`,
		`command -V "./bin/k6"`:      `command -V "./bin/k6"`,
		`command -p -v "./bin/k6"`:   `command -p -v "./bin/k6"`,
		`taskset -p 0x3 1234`:        `taskset -p 0x3 1234`,
		`taskset -pc 5-7 1234`:       `taskset -pc 5-7 1234`,
		`sudo -b "./bin/k6" run`:     `sudo -b "./bin/k6" run`,
		`env -S "./bin/k6 run t.js"`: `env -S "./bin/k6 run t.js"`,
		`nohup "./bin/k6" run x`:     `nohup "./bin/k6" run x`,
		// An option the table doesn't know leaves the command as it is.
		`timeout --frobnicate 5 "./bin/k6" run`: `timeout --frobnicate 5 "./bin/k6" run`,
		`taskset -cp 5-7 "./bin/k6" run`:        `taskset -cp 5-7 "./bin/k6" run`,
		`sudo -Eu runner "./bin/k6" run`:        `sudo -Eu runner "./bin/k6" run`,
		`exec -cl "./bin/k6" run`:               `exec -cl "./bin/k6" run`,
		`sudo - "./bin/k6" run`:                 `sudo - "./bin/k6" run`,
		// Not wrappers.
		`"$SUDO" "./bin/k6" run`:      `"$SUDO" "./bin/k6" run`,
		`timeouts 10m "./bin/k6" run`: `timeouts 10m "./bin/k6" run`,
	} {
		assert.Equal(t, want, shell.NormalizeCommand(command), command)
		assert.Equal(t, []string{"k6", "run", "t.js"}, shell.Words("timeout 10m k6 run t.js"))
		assert.Empty(t, shell.Words("command -v k6"))
	}
}

func TestSwallowed(t *testing.T) {
	runs := func(command string) bool { return strings.Contains(command, "gitleaks") }
	for script, want := range map[string]bool{
		"gitleaks detect || true":                                                  true,
		"gitleaks detect || echo 'leaks found'":                                    true,
		"if ! gitleaks detect; then echo found; fi":                                true,
		"if gitleaks detect; then echo clean; fi":                                  true,
		"! gitleaks detect":                                                        true,
		"while ! gitleaks detect; do sleep 1; done":                                true,
		"set -e\ngitleaks detect --report r.json \\\n  || true":                    true,
		"gitleaks detect":                                                          false,
		"gitleaks detect || exit 1":                                                false,
		"gitleaks detect || exit $?":                                               false,
		"gitleaks detect || { echo 'leaks found'; exit 1; }":                       false,
		"gitleaks detect || false":                                                 false,
		"if ! gitleaks detect; then exit 1; fi":                                    false,
		"if ! gitleaks detect; then\n  failed=1\nfi\n[ -z \"$failed\" ] || exit 1": false,
		"if gitleaks detect; then echo clean; else return 1; fi":                   false,
		"gitleaks detect || true\ngitleaks detect --staged":                        false,
		"make || gitleaks detect":                                                  false,
		`if [ "$TOOL" = gitleaks ]; then make; fi`:                                 false,
		"pip install gitleaks || true":                                             false,
		"echo gitleaks || true":                                                    false,
		"# gitleaks detect || true":                                                false,
		"":                                                                         false,
		"if (gitleaks detect) { echo clean }":                                      true,
		"if (-not (gitleaks detect --report r.json)) { exit 1 }":                   false,
		"if(!(gitleaks detect)) { exit 1 }":                                        false,
		"if (gitleaks detect); then exit 1; fi":                                    false,
		// && splits a bash subshell condition, leaving "if (cd src" unclosed.
		"if (cd src && gitleaks detect); then echo clean; fi": false,
		"OUT=$(gitleaks detect)":                              false,
		"OUT=$(gitleaks detect) || true":                      true,
		"export OUT=$(gitleaks detect)":                       true,
		`echo "$(gitleaks detect)"`:                           true,

		"timeout 10m gitleaks detect || true":                   true,
		"sudo -E timeout 10m gitleaks detect":                   false,
		"if ! timeout 10m gitleaks detect; then echo found; fi": true,

		// Under set +e only the last command, or one whose $? is read, can fail.
		"set +e\ngitleaks detect\necho done":                                                  true,
		"set +eo pipefail\ngitleaks detect\nset -e\nmake":                                     true,
		"set +e\nOUT=$(gitleaks detect)\nset -e\necho \"$OUT\"":                               true,
		"set +e\ngitleaks detect\nrc=$?\ncase $rc in 0|1) echo report ;; *) exit $rc ;; esac": true,
		"set +e\ngitleaks detect":                                                             false,
		"set +e\ngitleaks detect\nrc=$?\necho done\nexit $rc":                                 false,
		"set +e\ngitleaks detect\nrc=$?\ncase $rc in 0) ;; *) exit $rc ;; esac":               false,
		"set +e\nset -e\ngitleaks detect\necho done":                                          false,
		"set +o errexit\nset -o errexit\ngitleaks detect\necho done":                          false,
		"set -e\ngitleaks detect\necho done":                                                  false,
		"set +e\nif true; then\n  gitleaks detect\nfi":                                        false,
		"set +e\ngitleaks detect | tee log\nstatus=${PIPESTATUS[0]}\nexit \"$status\"":        false,
		"set +e\ncase $MODE in\n  all) gitleaks detect ;;\nesac\nrc=$?\nexit $rc":             false,
	} {
		assert.Equal(t, want, shell.Swallowed(script, runs), script)
	}
}

func TestSubstitutions(t *testing.T) {
	for command, want := range map[string]struct {
		inner  []string
		masked bool
	}{
		"OUT=$(gitleaks detect --source .)":       {[]string{"gitleaks detect --source ."}, false},
		`OUT="$(cd src && gitleaks detect)" make`: {[]string{"cd src && gitleaks detect"}, true},
		`echo "found: $(gitleaks detect)"`:        {[]string{"gitleaks detect"}, true},
		"$(go env GOPATH)/bin/gosec ./...":        {nil, true},
		"gosec $(pwd) $((1 + 2))":                 {nil, true},
		"echo '$(gitleaks detect)'":               {nil, true},
		"cd $(Build.SourcesDirectory)":            {nil, true},
	} {
		inner, masked := shell.Substitutions(command)
		assert.Equal(t, want.inner, inner, command)
		assert.Equal(t, want.masked, masked, command)
	}
}

func TestEndsRelaxed(t *testing.T) {
	assert.True(t, shell.EndsRelaxed("set +e\nmake lint"))
	assert.True(t, shell.EndsRelaxed("set -e\nset +o errexit"))
	assert.False(t, shell.EndsRelaxed("set +e\nmake lint\nset -euo pipefail"))
	assert.False(t, shell.EndsRelaxed("make lint"))
}

func TestPassive(t *testing.T) {
	for command, want := range map[string]bool{
		"chmod +x bin/grype":              true,
		"python -m pip install bandit":    true,
		"uv tool install ruff":            true,
		"trivy --version":                 true,
		"npm run lint":                    false,
		"trivy fs --version-check .":      false,
		"go run ./cmd/gosec ./...":        false,
		"command -v shellcheck":           true,
		"readlink -f node_modules/vitest": true,
		"command shellcheck x.sh":         false,

		// A version or help check through a package runner runs no scan (#607).
		"npx eslint --version":                              true,
		"npx --yes eslint --help":                           true,
		"pnpm exec biome --version":                         true,
		"pnpm dlx @lhci/cli@0.15.1 --help":                  true,
		"yarn eslint --version":                             true,
		"bunx eslint -V":                                    true,
		"npm exec -- eslint --version":                      true,
		"uvx ruff --version":                                true,
		"uv run ty --version":                               true,
		"uv tool run ruff --version":                        true,
		"pipx run ruff --version":                           true,
		"poetry run ruff --version":                         true,
		"npx eslint .":                                      false,
		"pnpm exec eslint --max-warnings 0 .":               false,
		"uv run ruff check .":                               false,
		"npm run version":                                   false,
		"pnpm run version":                                  false,
		"yarn run version":                                  false,
		"pnpm changeset version":                            false,
		"npx lerna version":                                 false,
		"npx eslint --version-check .":                      false,
		"timeout 60 curl -sSL https://example.com/trivy.sh": true,
		"if ! timeout 60 curl -sSL https://example.com/x":   true,
		"command -v k6":                                     true,
		"timeout 10m trivy fs .":                            false,
		"sudo -E chmod +x ./trivy":                          true,
		"sudo apt-get install -y trivy":                     true,
		"nice -n 10 go install example.com/gosec@latest":    true,
		"env -i curl -sSL https://example.com/x":            true,
		"time trivy --version":                              true,
		"command -v":                                        true,
		"taskset -p 0x3 1234":                               true,
		"taskset -pc 5-7 1234":                              true,
		"sudo -b k6 run x":                                  true,
		"env":                                               true,
		"sudo -u runner trivy fs .":                         false,
		"xvfb-run -a npm test":                              false,
		"timeout --frobnicate 5 curl x":                     false,
		"taskset -cp 5-7 1234":                              false,
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
		`addlicense -c "OpenSSF Scorecard Authors" .`:             "addlicense -c .",
		`git commit -m "chore: bump golangci-lint"`:               "git commit -m",
		"ct lint-and-install --charts helm/trivy":                 "ct lint-and-install --charts",
		"cargo build --bin ruff":                                  "cargo build --bin",
		"gh release view --repo gitleaks/gitleaks --json tagName": "gh release view --repo --json tagName",
		"docker build -t ghcr.io/aquasecurity/trivy:dev .":        "docker build -t .",
		`bash -c "gitleaks detect --no-git"`:                      `bash -c "gitleaks detect --no-git"`,
		"docker run --rm -v \"$PWD:/src\" aquasec/trivy fs /src":  "docker run --rm -v aquasec/trivy fs /src",
		"python -m pyright src/requests/":                         "python -m pyright src/requests/",
		"npx eslint .":                                            "npx eslint .",
		"mvn org.owasp:dependency-check-maven:check":              "mvn org.owasp:dependency-check-maven:check",
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

func TestSwallowedReadsALongScriptOnce(t *testing.T) {
	script := strings.Repeat("if trivy fs .; then echo ok; fi\n", 2000) + "trivy fs . || true"
	start := time.Now()
	swallowed, kept := shell.SwallowedCommands(script), 0
	for _, command := range shell.Commands(script) {
		if !swallowed(command) {
			kept++
		}
	}
	all := shell.Swallowed(script, func(c string) bool { return c == "trivy fs ." })
	elapsed := time.Since(start)
	assert.Equal(t, 4001, kept)
	assert.True(t, all)
	assert.False(t, shell.SwallowedCommands(script+"\nexit 1")("if trivy fs ."))
	assert.Less(t, elapsed, 2*time.Second)
}

func TestLines(t *testing.T) {
	script := "#!/bin/bash\nset -e\n\n  # a comment\nruff check . || true\ngo test \\\n  ./...\ncat > config.yml <<EOF\ngolangci-lint: yes\nEOF\nif ! mypy .; then exit 1; fi"
	assert.Equal(t, []string{"set -e", "ruff check . || true", "go test    ./...", "cat > config.yml <<EOF", "if ! mypy .; then exit 1; fi"}, shell.Lines(script))
	assert.Empty(t, shell.Lines("\n# only\n"))
}
