package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	pause = 0
	backoff = time.Millisecond
	rateLimitFloor = time.Millisecond
}

// fakeSearch serves stars in descending order, 100 per page, honouring a
// "stars:1..N" ceiling, and records the queries it was asked.
func fakeSearch(t *testing.T, stars []int) *[]string {
	t.Helper()
	var queries []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		queries = append(queries, q)
		ceiling := 1 << 30
		if _, after, ok := strings.Cut(q, "stars:1.."); ok {
			ceiling, _ = strconv.Atoi(after)
		}
		var matching []candidate
		for i, s := range stars {
			if s <= ceiling {
				matching = append(matching, candidate{FullName: fmt.Sprintf("o/r%d", i), Stars: s})
			}
		}
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		start, end := min((page-1)*100, len(matching)), min(page*100, len(matching))
		_ = json.NewEncoder(w).Encode(map[string]any{"items": matching[start:end]})
	}))
	t.Cleanup(server.Close)
	old := apiBase
	apiBase = server.URL
	t.Cleanup(func() { apiBase = old })
	return &queries
}

func descending(n int) []int {
	stars := make([]int, n)
	for i := range stars {
		stars[i] = 100_000 - i
	}
	return stars
}

func TestSearchContinuesBelowTheLastStarCount(t *testing.T) {
	queries := fakeSearch(t, descending(2500))
	found, err := searchMostStarred(1500, "token")
	require.NoError(t, err)
	assert.Len(t, found, 1500)
	assert.Equal(t, "o/r0", found[0].FullName)
	assert.Equal(t, "o/r1499", found[1499].FullName, "no repository is skipped or repeated across queries")
	assert.Contains(t, *queries, "archived:false fork:false stars:1..99001")
}

func TestSearchStopsWhenResultsRunOut(t *testing.T) {
	fakeSearch(t, descending(150))
	found, err := searchMostStarred(1000, "")
	require.NoError(t, err)
	assert.Len(t, found, 150)
}

func TestSearchStopsWhenTheCeilingCannotMove(t *testing.T) {
	stars := make([]int, 1200)
	for i := range stars {
		stars[i] = 50
	}
	fakeSearch(t, stars)
	found, err := searchMostStarred(2000, "")
	require.NoError(t, err)
	assert.Len(t, found, 1000, "1,200 repositories with the same stars cannot be paged past 1,000")
}

func TestSearchRetriesARateLimit(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		if calls == 1 {
			w.Header().Set("Retry-After", "0")
			http.Error(w, "secondary rate limit", http.StatusForbidden)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"items": []candidate{{FullName: "o/r", Stars: 9}}})
	}))
	defer server.Close()
	old := apiBase
	apiBase = server.URL
	defer func() { apiBase = old }()

	found, err := searchMostStarred(1, "")
	require.NoError(t, err)
	assert.Equal(t, []candidate{{FullName: "o/r", Stars: 9}}, found)
	assert.Equal(t, 2, calls)
}

func TestSearchReportsErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "rate limited", http.StatusForbidden)
	}))
	defer server.Close()
	old := apiBase
	apiBase = server.URL
	defer func() { apiBase = old }()
	_, err := searchMostStarred(10, "")
	assert.ErrorContains(t, err, "403", "still refused after every retry")

	server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "broken", http.StatusInternalServerError)
	})
	_, err = searchMostStarred(10, "")
	assert.ErrorContains(t, err, "500", "other failures are not retried")

	apiBase = "://bad"
	_, err = searchMostStarred(10, "")
	assert.Error(t, err)

	apiBase = "http://127.0.0.1:1"
	_, err = searchMostStarred(10, "")
	assert.Error(t, err)
}

func TestLoadCandidatesSavesAndReuses(t *testing.T) {
	fakeSearch(t, descending(30))
	path := filepath.Join(t.TempDir(), "candidates.json")

	first, err := loadCandidates(path, 10, "")
	require.NoError(t, err)
	assert.Len(t, first, 20, "twice n, since many repositories have no CI")

	apiBase = "http://127.0.0.1:1"
	again, err := loadCandidates(path, 10, "")
	require.NoError(t, err, "the saved list is read without asking GitHub again")
	assert.Equal(t, first, again)

	_, err = loadCandidates(t.TempDir(), 10, "")
	assert.Error(t, err, "a directory where the list belongs")
	_, err = loadCandidates(filepath.Join(t.TempDir(), "missing", "c.json"), 10, "")
	assert.Error(t, err, "the search fails without GitHub")
}

// fakeZanadir is a script standing in for the binary: o/broken fails, o/busy
// is rate limited once, o/nobody has no CI, everything else has GitHub CI.
func fakeZanadir(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fake zanadir is a shell script")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "zanadir")
	state := filepath.Join(dir, "busy-once")
	require.NoError(t, os.WriteFile(script, []byte(`#!/bin/sh
case "$3" in
  o/broken) echo "Error: scan repo failed: boom"; exit 1 ;;
  o/garbled) echo "not json" ;;
  o/busy) if [ ! -f `+state+` ]; then touch `+state+`; echo "Error: downloading: 429 Too Many Requests"; exit 1; fi
          echo '{"score":{"covered":1,"total":10},"platforms":["github"],"uncovered":[],"weakened":[],"notApplicable":[]}' ;;
  o/nobody) echo '{"score":{"covered":0,"total":10},"platforms":[],"uncovered":[],"weakened":[],"notApplicable":[]}' ;;
  *) echo '{"score":{"covered":2,"total":10},"platforms":["github"],"uncovered":[{"id":"SAST"}],"weakened":[{"category":"SCA","verdict":"advisory","location":".github/dependabot.yml"}],"notApplicable":["IaC Security"]}' ;;
esac
`), 0o755))
	return script
}

func names(list ...string) []candidate {
	var out []candidate
	for _, n := range list {
		out = append(out, candidate{FullName: n})
	}
	return out
}

func TestScanUntilStopsAtNWithCIAndResumes(t *testing.T) {
	zanadir, dir := fakeZanadir(t), t.TempDir()
	candidates := names("o/a", "o/nobody", "o/broken", "o/busy", "o/garbled", "o/b", "o/c", "o/d")

	scanUntil(zanadir, candidates, 3, dir, 2, "token")
	_, ok := readResult(dir, "o/busy")
	assert.True(t, ok, "a rate-limited scan is retried")
	assert.FileExists(t, rawPath(dir, "o/broken")+".err")
	assert.FileExists(t, rawPath(dir, "o/garbled")+".err", "output that is not JSON is a failure")
	assert.NoFileExists(t, rawPath(dir, "o/d"), "scanning stops once n repositories with CI are in")

	require.NoError(t, os.Remove(zanadir), "a second run must not scan anything again")
	scanUntil(zanadir, candidates[:6], 3, dir, 2, "token")
}

func TestAggregate(t *testing.T) {
	zanadir, dir := fakeZanadir(t), t.TempDir()
	candidates := names("o/a", "o/nobody", "o/broken", "o/never-scanned", "o/b")
	for _, c := range []string{"o/a", "o/nobody", "o/broken", "o/b"} {
		scanOne(zanadir, c, dir, "")
	}
	weak := `{"score":{"covered":3,"total":12},"platforms":["github","gitlab"],"uncovered":[],"weakened":[{"category":"Linter","verdict":"partial","location":".github/workflows/lint.yml"},{"category":"SAST","verdict":"advisory"},{"category":"Unknown","verdict":"advisory"}],"notApplicable":[]}`
	require.NoError(t, os.WriteFile(rawPath(dir, "o/weak"), []byte(weak), 0o600))
	candidates = append(candidates, names("o/weak", "o/after-n")...)
	require.NoError(t, os.WriteFile(rawPath(dir, "o/after-n"), []byte(weak), 0o600))

	s := aggregate(dir, candidates, 3, time.Date(2026, 10, 17, 0, 0, 0, 0, time.UTC))
	assert.Equal(t, "2026-10-17", s.Date)
	assert.Equal(t, 3, s.Repositories, "o/after-n is past n")
	assert.Equal(t, 4, s.Scanned)
	assert.Equal(t, 1, s.WithoutCI)
	assert.Equal(t, 1, s.Failed)
	assert.Equal(t, map[string]int{"github": 3, "gitlab": 1}, s.Platforms)
	assert.InDelta(t, 0.2, s.MedianCoverage, 0.001)
	assert.Equal(t, 1, s.WithWeakened, "a dependency bot alone is not a weakened pipeline")

	byName := map[string]categoryStat{}
	for _, c := range s.Categories {
		byName[c.Category] = c
	}
	assert.Equal(t, categoryStat{Category: "SCA", Applicable: 3, Covered: 3, BotOnly: 2}, byName["SCA"])
	assert.Equal(t, categoryStat{Category: "SAST", Applicable: 3, Covered: 1, Weakened: 1, Advisory: 1}, byName["SAST"])
	assert.Equal(t, categoryStat{Category: "Linter", Applicable: 3, Covered: 3, Weakened: 1, Partial: 1}, byName["Linter"])
	assert.Equal(t, 1, byName["IaC Security"].Applicable)
}

func TestMedianAndPercent(t *testing.T) {
	assert.Zero(t, median(nil))
	assert.InDelta(t, 0.5, median([]float64{0.9, 0.1, 0.5}), 0.001)
	assert.InDelta(t, 0.4, median([]float64{0.2, 0.6}), 0.001)
	assert.Equal(t, "-", percent(1, 0))
	assert.Equal(t, "33%", percent(1, 3))
}

func TestIsBotConfig(t *testing.T) {
	for _, path := range []string{".github/dependabot.yml", ".github/dependabot.yaml", "renovate.json", ".github/renovate.json5", ".renovaterc.json"} {
		assert.True(t, isBotConfig(path), path)
	}
	assert.False(t, isBotConfig(".github/workflows/ci.yml"))
	assert.False(t, isBotConfig(""))
}

func TestRunEndToEnd(t *testing.T) {
	fakeSearch(t, descending(10))
	out := filepath.Join(t.TempDir(), "report")
	require.NoError(t, run(fakeZanadir(t), 3, out, 2, "token"))

	md, err := os.ReadFile(filepath.Join(out, "summary.md"))
	require.NoError(t, err)
	assert.Contains(t, string(md), "3 of the most-starred GitHub repositories with CI")
	assert.Contains(t, string(md), "| SCA | 100% | 0% | 0 | 0 | 100% |")
	assert.Contains(t, string(md), "| github | 3 |")
	assert.NotContains(t, string(md), "o/r", "the summary never names a repository")

	var s summary
	data, err := os.ReadFile(filepath.Join(out, "summary.json"))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, &s))
	assert.Equal(t, 3, s.Repositories)
}

func TestRunReportsWhatItCannotWrite(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(file, nil, 0o600))
	assert.Error(t, run("zanadir", 1, file, 1, ""), "the output path is a file")

	apiBase = "http://127.0.0.1:1"
	defer func() { apiBase = "https://api.github.com" }()
	assert.Error(t, run("zanadir", 1, t.TempDir(), 1, ""), "the search fails")

	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "raw"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "candidates.json"), []byte("[]"), 0o600))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "summary.json"), 0o755))
	assert.Error(t, run("zanadir", 1, dir, 1, ""), "summary.json cannot be written")

	require.NoError(t, os.Remove(filepath.Join(dir, "summary.json")))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "summary.md"), 0o755))
	assert.Error(t, run("zanadir", 1, dir, 1, ""), "summary.md cannot be written")
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, fmt.Errorf("disk full") }

func TestWriteMarkdownReportsAWriteError(t *testing.T) {
	assert.ErrorContains(t, writeMarkdown(failingWriter{}, summary{Platforms: map[string]int{"github": 2, "gitlab": 1}}), "disk full")
}

func TestCLI(t *testing.T) {
	assert.Equal(t, 2, cli([]string{"-no-such-flag"}, ""))

	fakeSearch(t, descending(4))
	out := filepath.Join(t.TempDir(), "r")
	assert.Equal(t, 0, cli([]string{"-zanadir", fakeZanadir(t), "-n", "2", "-workers", "1", "-out", out}, "token"))
	assert.FileExists(t, filepath.Join(out, "summary.md"))

	assert.Equal(t, 1, cli([]string{"-out", "/dev/null/x"}, ""))
}

func TestMainExitsWithTheCLIStatus(t *testing.T) {
	code := -1
	exit = func(c int) { code = c }
	args := os.Args
	defer func() { exit, os.Args = os.Exit, args }()

	os.Args = []string{"stateofci", "-no-such-flag"}
	main()
	assert.Equal(t, 2, code)
}
