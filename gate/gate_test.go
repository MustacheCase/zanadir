package gate

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MustacheCase/zanadir/models"
	"github.com/MustacheCase/zanadir/remote"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// github serves canned bodies by path, answering 404 for anything else.
func github(t *testing.T, bodies map[string]string) *[]string {
	t.Helper()
	var auth []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = append(auth, r.Header.Get("Authorization"))
		body, ok := bodies[r.URL.Path+"?"+r.URL.RawQuery]
		if !ok {
			body, ok = bodies[r.URL.Path]
		}
		switch {
		case !ok:
			http.NotFound(w, r)
		case body == "500":
			w.WriteHeader(http.StatusInternalServerError)
		default:
			_, _ = w.Write([]byte(body))
		}
	}))
	t.Cleanup(server.Close)
	old := apiBase
	apiBase = server.URL
	t.Cleanup(func() { apiBase = old })
	return &auth
}

var repo = remote.Repository{Owner: "o", Name: "r"}

func TestReadCollectsRequiredChecks(t *testing.T) {
	auth := github(t, map[string]string{
		"/repos/o/r": `{"default_branch": "release/v1"}`,
		"/repos/o/r/rules/branches/release/v1": `[
			{"type": "pull_request"},
			{"type": "code_scanning", "parameters": {"code_scanning_tools": [{"tool": "zizmor"}, {"tool": "Snyk Open Source"},
				{"tool": "CodeQL", "alerts_threshold": "none", "security_alerts_threshold": "none"},
				{"tool": "Trivy", "alerts_threshold": "none", "security_alerts_threshold": "high_or_higher"}]}},
			{"type": "required_status_checks", "parameters": {"required_status_checks": [
				{"context": "Unit tests", "integration_id": 15368}, {"context": "DCO"}]}}]`,
		"/repos/o/r/branches/release/v1": `{"protection": {"required_status_checks": {
			"enforcement_level": "non_admins", "checks": [{"context": "lint", "app_id": 15368}, {"context": "ci/prow", "app_id": null}]}}}`,
		"/repos/o/r/rulesets": `[{"id": 1}, {"id": 2}, {"id": 3}, {"id": 4}, {"id": 5}]`,
		"/repos/o/r/rulesets/1": `{"name": "nowhere", "target": "branch", "enforcement": "active",
			"conditions": {"ref_name": {"include": []}},
			"rules": [{"type": "required_status_checks", "parameters": {"required_status_checks": [{"context": "Run tests"}]}}]}`,
		"/repos/o/r/rulesets/2": `{"name": "trial", "target": "branch", "enforcement": "evaluate",
			"conditions": {"ref_name": {"include": ["~DEFAULT_BRANCH"]}},
			"rules": [{"type": "required_status_checks", "parameters": {"required_status_checks": [{"context": "e2e"}]}}]}`,
		"/repos/o/r/rulesets/3": `{"name": "fine", "target": "branch", "enforcement": "active",
			"conditions": {"ref_name": {"include": ["~DEFAULT_BRANCH"]}},
			"rules": [{"type": "required_status_checks", "parameters": {"required_status_checks": [{"context": "Unit tests"}]}}]}`,
		"/repos/o/r/rulesets/4": `{"name": "tags", "target": "tag", "enforcement": "active", "rules": [
			{"type": "required_status_checks", "parameters": {"required_status_checks": [{"context": "x"}]}}]}`,
		"/repos/o/r/rulesets/5": `{"name": "reviews only", "target": "branch", "enforcement": "active", "rules": [{"type": "pull_request"}]}`,
	})

	g, err := Read(repo, "secret")
	require.NoError(t, err)
	assert.Equal(t, "release/v1", g.Branch)
	assert.Equal(t, []Check{{"Unit tests", 15368}, {"DCO", 0}, {"lint", 15368}, {"ci/prow", 0}}, g.Checks)
	assert.Equal(t, []string{"zizmor", "Snyk Open Source", "Trivy"}, g.Scanning, "both thresholds at none block nothing")
	assert.Equal(t, []string{
		`ruleset "nowhere" requires "Run tests" but applies to no branch`,
		`ruleset "trial" requires "e2e" but is evaluate, so it blocks nothing`,
	}, g.Problems)
	assert.Equal(t, "Bearer secret", (*auth)[0])
}

func TestReadWithoutProtection(t *testing.T) {
	auth := github(t, map[string]string{
		"/repos/o/r":                     `{"default_branch": "main"}`,
		"/repos/o/r/rules/branches/main": `[]`,
		"/repos/o/r/branches/main":       `{"protection": {"required_status_checks": {"enforcement_level": "off", "checks": [{"context": "stale"}]}}}`,
	})
	g, err := Read(repo, "")
	require.NoError(t, err)
	assert.Empty(t, g.Checks, "checks protection does not enforce are not required")
	assert.Empty(t, g.Problems, "unreadable rulesets are skipped")
	assert.Empty(t, (*auth)[0], "a public repository needs no token")
}

func TestReadFailures(t *testing.T) {
	github(t, map[string]string{})
	_, err := Read(repo, "")
	assert.ErrorContains(t, err, "github.com/o/r: not found")

	github(t, map[string]string{"/repos/o/r": `{"default_branch": "main"}`, "/repos/o/r/rules/branches/main": "500"})
	_, err = Read(repo, "")
	assert.ErrorContains(t, err, "rules on main: GitHub answered 500")

	github(t, map[string]string{"/repos/o/r": `{"default_branch": "main"}`, "/repos/o/r/rules/branches/main": `[]`})
	_, err = Read(repo, "")
	assert.ErrorContains(t, err, "protection of main")

	github(t, map[string]string{"/repos/o/r": `not json`})
	_, err = Read(repo, "")
	assert.Error(t, err)

	old := apiBase
	apiBase = "http://127.0.0.1:0"
	t.Cleanup(func() { apiBase = old })
	_, err = Read(repo, "")
	assert.Error(t, err)

	apiBase = "://bad"
	_, err = Read(repo, "")
	assert.Error(t, err)
}

func TestRequires(t *testing.T) {
	g := &Gate{Checks: []Check{{Context: "Unit tests"}, {Context: "build (ubuntu-latest)"},
		{Context: "codeql / Analyze (go)"}, {Context: "Analyze (ruby)"}, {Context: "a.b+c"},
		{Context: "DCO", AppID: 1861}, {Context: "lint", AppID: actionsApp}, {Context: "scan / gitleaks"}}}
	caller := func(check string) *models.Job {
		return &models.Job{Check: check, Package: "./.github/workflows/codeql.yml"}
	}
	for _, tt := range []struct {
		job  *models.Job
		want bool
	}{
		{&models.Job{Check: "Unit tests"}, true},
		{&models.Job{Check: "build"}, true},
		{caller("codeql"), true},
		{&models.Job{Check: "codeql"}, false},
		{&models.Job{Check: "scan"}, false},
		{&models.Job{Check: "Analyze (${{ matrix.language }})"}, true},
		{&models.Job{Check: "${{ matrix.os }} tests"}, true},
		{&models.Job{Check: "${{ matrix.os }} e2e"}, false},
		{&models.Job{Check: "${{ matrix.tool }}"}, false},
		{&models.Job{Check: "Unit"}, false},
		{&models.Job{Check: "Unit tests extra"}, false},
		{&models.Job{Check: "a.b+c"}, true},
		{&models.Job{Check: "axb+c"}, false},
		{&models.Job{Check: "DCO"}, false},
		{&models.Job{Check: "lint"}, true},
	} {
		assert.Equal(t, tt.want, g.Requires(tt.job), "%+v", tt.job)
	}
}

func TestUnproduced(t *testing.T) {
	g := &Gate{Checks: []Check{{"Unit tests", actionsApp}, {"Gone", actionsApp}, {"DCO", 1861}, {"Any app", 0}}}
	g.Unproduced([]*models.Job{{Check: "Unit tests"}})
	assert.Equal(t, []string{`required check "Gone" is reported by no workflow job, so pull requests wait for it`}, g.Problems)

	g = &Gate{Checks: []Check{{"Gone", actionsApp}}}
	g.Unproduced([]*models.Job{{Check: "${{ matrix.name }}"}})
	assert.Empty(t, g.Problems, "a job named by expressions may report it")
}

func TestOrigin(t *testing.T) {
	checkout := func(config string) string {
		dir := t.TempDir()
		require.NoError(t, os.Mkdir(filepath.Join(dir, ".git"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, ".git", "config"), []byte(config), 0o600))
		return dir
	}
	for config, want := range map[string]string{
		"[core]\n\tbare = false\n[remote \"origin\"]\n\turl = https://github.com/o/r.git\n\tfetch = +refs/heads/*\n": "github.com/o/r",
		"[remote \"origin\"]\n\turl = git@github.com:o/r.git\n[branch \"main\"]\n\tremote = origin\n":                "github.com/o/r",
		"[remote \"origin\"]\n\turl = ssh://git@github.com/o/r\n":                                                    "github.com/o/r",
	} {
		got, err := Origin(checkout(config))
		require.NoError(t, err, config)
		assert.Equal(t, want, got.String())
	}

	_, err := Origin(checkout("[remote \"upstream\"]\n\turl = https://github.com/o/r\n"))
	assert.ErrorContains(t, err, "no origin remote")
	_, err = Origin(checkout("[remote \"origin\"]\n\tfetch = x\n[core]\n\turl = https://github.com/o/r\n"))
	assert.ErrorContains(t, err, "no url")
	_, err = Origin(checkout("[remote \"origin\"]\n\turl = https://gitlab.com/o/r\n"))
	assert.Error(t, err)
	_, err = Origin(t.TempDir())
	assert.Error(t, err)
}

func TestBlocks(t *testing.T) {
	g := &Gate{Checks: []Check{{Context: "Unit tests"}}, Scanning: []string{"zizmor", "Snyk Open Source", ""}}
	for job, want := range map[*models.Job]bool{
		{Check: "Unit tests"}: true,
		{Check: "audit", Package: "zizmorcore/zizmor-action"}:         true,
		{Check: "deps", Run: "snyk test --sarif-file-output=s.sarif"}: true,
		{Check: "lint", Run: "golangci-lint run"}:                     false,
		{Check: "lint"}: false,
	} {
		assert.Equal(t, want, g.Blocks(job), job.Check)
	}
	assert.False(t, g.Scans(&models.Job{Check: "Unit tests"}), "a required check is not code scanning")
	assert.True(t, g.Scans(&models.Job{Run: "zizmor --format sarif ."}))
}

// GitHub lists 30 items a page by default; a repository with more rules
// lost the ones past the first page.
func TestReadFollowsPages(t *testing.T) {
	var page1 []string
	for i := range 100 {
		page1 = append(page1, fmt.Sprintf(`{"type": "deletion", "ruleset_id": %d}`, i))
	}
	github(t, map[string]string{
		"/repos/o/r": `{"default_branch": "main"}`,
		"/repos/o/r/rules/branches/main?per_page=100&page=1": "[" + strings.Join(page1, ",") + "]",
		"/repos/o/r/rules/branches/main?per_page=100&page=2": `[{"type": "required_status_checks", "parameters": {"required_status_checks": [{"context": "Unit tests"}]}}]`,
		"/repos/o/r/branches/main":                           `{}`,
	})
	g, err := Read(repo, "")
	require.NoError(t, err)
	assert.Equal(t, []Check{{Context: "Unit tests"}}, g.Checks)
}

func TestDefaultBranch(t *testing.T) {
	dir := t.TempDir()
	assert.Empty(t, DefaultBranch(dir), "no origin/HEAD, as in most CI checkouts")
	head := filepath.Join(dir, ".git", "refs", "remotes", "origin", "HEAD")
	require.NoError(t, os.MkdirAll(filepath.Dir(head), 0o755))
	require.NoError(t, os.WriteFile(head, []byte("ref: refs/remotes/origin/develop\n"), 0o600))
	assert.Equal(t, "develop", DefaultBranch(dir))
	require.NoError(t, os.WriteFile(head, []byte("0123456789abcdef\n"), 0o600))
	assert.Empty(t, DefaultBranch(dir), "a detached origin/HEAD names no branch")
}

// A worktree's .git file names its git directory, whose commondir names the
// one holding the remotes; a submodule's names its git directory directly.
func TestRemotesAreReadThroughGitLinks(t *testing.T) {
	base := t.TempDir()
	write := func(path, content string) {
		t.Helper()
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	}
	main := filepath.Join(base, "main", ".git")
	write(filepath.Join(main, "config"), "[remote \"origin\"]\n\turl = https://github.com/o/r.git\n")
	write(filepath.Join(main, "refs", "remotes", "origin", "HEAD"), "ref: refs/remotes/origin/develop\n")
	write(filepath.Join(main, "worktrees", "wt", "commondir"), "../..\n")
	write(filepath.Join(base, "wt", ".git"), "gitdir: "+filepath.Join(main, "worktrees", "wt")+"\n")

	modules := filepath.Join(base, "super", ".git", "modules", "sub")
	write(filepath.Join(modules, "config"), "[remote \"origin\"]\n\turl = git@github.com:o/sub.git\n")
	write(filepath.Join(base, "super", "sub", ".git"), "gitdir: ../.git/modules/sub\n")

	for dir, want := range map[string]string{"main": "github.com/o/r", "wt": "github.com/o/r", "super/sub": "github.com/o/sub"} {
		got, err := Origin(filepath.Join(base, dir))
		require.NoError(t, err, dir)
		assert.Equal(t, want, got.String(), dir)
	}
	assert.Equal(t, "develop", DefaultBranch(filepath.Join(base, "wt")), "a worktree shares the main checkout's refs")
	assert.Empty(t, DefaultBranch(filepath.Join(base, "super", "sub")))

	write(filepath.Join(base, "odd", ".git"), "not a link\n")
	_, err := Origin(filepath.Join(base, "odd"))
	assert.Error(t, err)
}

func TestReadStopsAfterTooManyPages(t *testing.T) {
	defer func(pages int) { maxPages = pages }(maxPages)
	maxPages = 1
	var page1 []string
	for i := range 100 {
		page1 = append(page1, fmt.Sprintf(`{"type": "deletion", "ruleset_id": %d}`, i))
	}
	github(t, map[string]string{
		"/repos/o/r": `{"default_branch": "main"}`,
		"/repos/o/r/rules/branches/main?per_page=100&page=1": "[" + strings.Join(page1, ",") + "]",
		"/repos/o/r/branches/main":                           `{}`,
	})
	_, err := Read(repo, "")
	assert.ErrorContains(t, err, "more than 1 pages")
}

func TestReadStopsAtTheSizeLimit(t *testing.T) {
	defer func(limit int64) { maxJSONBytes = limit }(maxJSONBytes)
	maxJSONBytes = 8
	github(t, map[string]string{"/repos/o/r": `{"default_branch": "main"}`})
	_, err := Read(repo, "")
	assert.Error(t, err)
}
