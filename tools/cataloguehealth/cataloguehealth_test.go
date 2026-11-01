package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var now = time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)

type fakeRepo struct {
	archived bool
	pushed   string
	released string
	tags     map[string]string
	commits  map[string]string
}

// fakeGitHub serves the repositories given, 404s anything else, and fails
// any path listed in broken.
func fakeGitHub(t *testing.T, repos map[string]fakeRepo, broken ...string) *[]string {
	t.Helper()
	var auth []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = append(auth, r.Header.Get("Authorization"))
		for _, b := range broken {
			if r.URL.Path == b {
				w.WriteHeader(http.StatusBadGateway)
				return
			}
		}
		parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/repos/"), "/", 3)
		repo, ok := repos[parts[0]+"/"+parts[1]]
		if !ok {
			http.NotFound(w, r)
			return
		}
		rest := ""
		if len(parts) == 3 {
			rest = parts[2]
		}
		var body any
		switch {
		case rest == "":
			body = map[string]any{"archived": repo.archived, "pushed_at": timestamp(repo.pushed)}
		case rest == "releases/latest":
			if repo.released == "" {
				http.NotFound(w, r)
				return
			}
			body = map[string]any{"published_at": repo.released}
		case rest == "git/matching-refs/tags":
			var refs []map[string]string
			for tag := range repo.tags {
				refs = append(refs, map[string]string{"ref": "refs/tags/" + tag})
			}
			body = refs
		case strings.HasPrefix(rest, "commits/"):
			ref := strings.TrimPrefix(rest, "commits/")
			body = map[string]any{"sha": repo.commits[ref], "commit": map[string]any{"committer": map[string]any{"date": repo.tags[ref]}}}
		}
		_ = json.NewEncoder(w).Encode(body)
	}))
	t.Cleanup(server.Close)
	old := apiBase
	apiBase = server.URL
	t.Cleanup(func() { apiBase = old })
	return &auth
}

// timestamp is null when unset, as GitHub sends it.
func timestamp(at string) any {
	if at == "" {
		return nil
	}
	return at
}

func catalogue(t *testing.T, suggestions, templates string, skeleton ...string) string {
	t.Helper()
	root := t.TempDir()
	for path, content := range map[string]string{"suggester/suggestions.yaml": suggestions, "fixer/templates.yaml": templates, "fixer/fixer.go": strings.Join(skeleton, "")} {
		require.NoError(t, os.MkdirAll(filepath.Join(root, filepath.Dir(path)), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(root, path), []byte(content), 0o600))
	}
	return root
}

const suggestions = `categories:
  - id: sast
    suggestions:
      - name: Alive
        repository: https://github.com/o/alive
      - name: Gone
        repository: https://github.com/o/gone
      - name: Frozen
        repository: https://github.com/o/frozen/
      - name: Quiet
        repository: https://github.com/o/quiet
      - name: Untagged
        repository: https://github.com/o/untagged
      - name: Docs
        repository: https://example.com/docs
      - name: Alive again
        repository: https://github.com/o/alive
`

const templates = `templates:
  - tool: Current
    step: |
      - uses: o/action/sub@v3
      - uses: o/action@v3
  - tool: Behind
    step: "- uses: o/action@v2"
  - tool: Branch
    step: "- uses: o/action@main"
  - tool: Missing
    step: "- uses: o/nothing@v1"
  - tool: CLI
    step: "- run: gitleaks git"
  - tool: Commit
    step: "- uses: o/action@` + v31 + ` # v3.1.0"
  - tool: Relabelled
    step: "- uses: o/action@` + v31 + ` # v3"
  - tool: Unlabelled
    step: "- uses: o/action@` + v31 + `"
`

const v31 = "3a1c0de5e4f0a1b2c3d4e5f60718293a4b5c6d7e"

func healthyAndNot() map[string]fakeRepo {
	return map[string]fakeRepo{
		"o/alive":    {released: "2026-09-01T00:00:00Z"},
		"o/frozen":   {archived: true},
		"o/quiet":    {released: "2024-01-01T00:00:00Z", tags: map[string]string{"v1.0.0": "2024-01-01T00:00:00Z", "v1.1.0-rc1": "2026-01-01T00:00:00Z"}},
		"o/untagged": {pushed: "2023-05-29T00:00:00Z"},
		"o/action": {released: "2020-01-01T00:00:00Z", tags: map[string]string{"v2": "2026-08-01T00:00:00Z", "v3": "2026-08-01T00:00:00Z", "v3.1.0": "2026-09-01T00:00:00Z", "latest": "2026-10-01T00:00:00Z"},
			commits: map[string]string{"v3.1.0": v31, "v3": "9b8a7f6e5d4c3b2a19087f6e5d4c3b2a19087f6e"}},
	}
}

func TestCheckJudgesEveryRecommendedToolAndPin(t *testing.T) {
	fakeGitHub(t, healthyAndNot())
	findings, err := check(catalogue(t, suggestions, templates, `b.WriteString("      - uses: o/action@v2\n")`, `"- uses: o/frozen@v1"`), "", now)
	require.NoError(t, err)
	assert.Equal(t, []finding{
		{Broken: true, Source: "suggestion", Tool: "Gone", Repo: "o/gone", Detail: "the repository does not exist"},
		{Broken: true, Source: "suggestion", Tool: "Frozen", Repo: "o/frozen", Detail: "the repository is archived"},
		{Source: "suggestion", Tool: "Quiet", Repo: "o/quiet", Detail: "no release since 2024-01-01"},
		{Source: "suggestion", Tool: "Untagged", Repo: "o/untagged", Detail: "no release or push since 2023-05-29"},
		{Source: "fix template", Tool: "Behind", Repo: "o/action", Detail: "pinned to v2; v3 is out"},
		{Broken: true, Source: "fix template", Tool: "Branch", Repo: "o/action", Detail: "@main is not a tag"},
		{Broken: true, Source: "fix template", Tool: "Missing", Repo: "o/nothing", Detail: "the repository does not exist"},
		{Broken: true, Source: "fix template", Tool: "Relabelled", Repo: "o/action", Detail: "@3a1c0de is not v3"},
		{Broken: true, Source: "fix template", Tool: "Unlabelled", Repo: "o/action", Detail: "@3a1c0de is not labelled with a tag"},
		{Source: "fix workflow", Tool: "o/action", Repo: "o/action", Detail: "pinned to v2; v3 is out"},
		{Broken: true, Source: "fix workflow", Tool: "o/frozen", Repo: "o/frozen", Detail: "the repository is archived"},
	}, findings, "an old release is rescued by a newer tag, but not by a pre-release; a repository is read once and reported once")
}

func TestCheckReportsAFailedRequest(t *testing.T) {
	for _, path := range []string{"/repos/o/alive", "/repos/o/alive/releases/latest", "/repos/o/quiet/git/matching-refs/tags", "/repos/o/quiet/commits/v1.0.0", "/repos/o/action/commits/v3", "/repos/o/action", "/repos/o/skeleton"} {
		fakeGitHub(t, healthyAndNot(), path)
		_, err := check(catalogue(t, suggestions, templates, "uses: o/skeleton@v1"), "", now)
		assert.ErrorContains(t, err, "502 Bad Gateway for "+path)
	}
}

func TestCheckReportsAnUnreadableCatalogue(t *testing.T) {
	fakeGitHub(t, nil)
	_, err := check(catalogue(t, "categories: [", templates), "", now)
	assert.ErrorContains(t, err, "suggestions.yaml")
	_, err = check(catalogue(t, "", "templates: ["), "", now)
	assert.ErrorContains(t, err, "templates.yaml")
	_, err = check(t.TempDir(), "", now)
	assert.ErrorIs(t, err, os.ErrNotExist)
	root := catalogue(t, "", "")
	require.NoError(t, os.Remove(filepath.Join(root, "fixer", "fixer.go")))
	_, err = check(root, "", now)
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestGetFailsOnABadURLOrConnection(t *testing.T) {
	old := apiBase
	t.Cleanup(func() { apiBase = old })
	gh := &github{}
	apiBase = "http://bad\x7f"
	_, err := gh.get("/x", nil)
	assert.Error(t, err)
	server := httptest.NewServer(http.NotFoundHandler())
	server.Close()
	apiBase = server.URL
	_, err = gh.get("/x", nil)
	assert.Error(t, err)
}

func TestRenderGroupsBrokenBeforeOutOfDate(t *testing.T) {
	report := render([]finding{
		{Source: "suggestion", Tool: "Quiet", Repo: "o/quiet", Detail: "no release since 2024-01-01"},
		{Broken: true, Source: "fix template", Tool: "Gone", Repo: "o/gone", Detail: "the repository does not exist"},
	})
	broken, outOfDate := strings.Index(report, "## Broken"), strings.Index(report, "## Out of date")
	assert.Positive(t, broken)
	assert.Greater(t, outOfDate, broken)
	assert.Contains(t, report, "| fix template | Gone | o/gone | the repository does not exist |")
	assert.NotContains(t, render(nil), "##")
}

func TestCLIWritesTheReportOnlyWhenThereAreProblems(t *testing.T) {
	auth := fakeGitHub(t, healthyAndNot())
	out := filepath.Join(t.TempDir(), "report.md")
	assert.Equal(t, 0, cli([]string{"-root", catalogue(t, suggestions, templates), "-out", out}, "secret", now, nil))
	report, err := os.ReadFile(out)
	require.NoError(t, err)
	assert.Contains(t, string(report), "## Broken")
	assert.Contains(t, *auth, "Bearer secret")

	healthy := catalogue(t, "categories: []", "templates: []")
	assert.Equal(t, 0, cli([]string{"-root", healthy, "-out", out}, "", now, nil))
	report, err = os.ReadFile(out)
	require.NoError(t, err)
	assert.Empty(t, report, "an empty report tells the workflow to close the issue")

	var stdout bytes.Buffer
	assert.Equal(t, 0, cli([]string{"-root", healthy}, "", now, &stdout))
	assert.Contains(t, stdout.String(), "healthy")
}

func TestCLIFailures(t *testing.T) {
	fakeGitHub(t, nil)
	assert.Equal(t, 2, cli([]string{"-bogus"}, "", now, nil))
	assert.Equal(t, 1, cli([]string{"-root", t.TempDir()}, "", now, nil))
	healthy := catalogue(t, "categories: []", "templates: []")
	assert.Equal(t, 1, cli([]string{"-root", healthy, "-out", filepath.Join(t.TempDir(), "no", "such", "dir")}, "", now, nil))
}

func TestMain(t *testing.T) {
	old, oldArgs := exit, os.Args
	t.Cleanup(func() { exit, os.Args = old, oldArgs })
	fakeGitHub(t, nil)
	code := -1
	exit = func(c int) { code = c }
	os.Args = []string{"cataloguehealth", "-root", catalogue(t, "categories: []", "templates: []"), "-out", filepath.Join(t.TempDir(), "r.md")}
	main()
	assert.Equal(t, 0, code)
}
