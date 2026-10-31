package remote

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse(t *testing.T) {
	for in, want := range map[string]Repository{
		"github.com/cli/cli":                  {Owner: "cli", Name: "cli"},
		"https://github.com/cli/cli.git":      {Owner: "cli", Name: "cli"},
		"http://GitHub.com/psf/requests/":     {Owner: "psf", Name: "requests"},
		"cli/cli":                             {Owner: "cli", Name: "cli"},
		"github.com/cli/cli@v2.60.0":          {Owner: "cli", Name: "cli", Ref: "v2.60.0"},
		"expressjs/express@feature/x-1":       {Owner: "expressjs", Name: "express", Ref: "feature/x-1"},
		" github.com/a-b/c_d.e@0123abc ":      {Owner: "a-b", Name: "c_d.e", Ref: "0123abc"},
		"github.com/MustacheCase/zanadir.git": {Owner: "MustacheCase", Name: "zanadir"},
		"HTTPS://github.com/cli/cli":          {Owner: "cli", Name: "cli"},
		"git@github.com:cli/cli.git":          {Owner: "cli", Name: "cli"},
		"ssh://git@github.com/cli/cli":        {Owner: "cli", Name: "cli"},
	} {
		got, err := Parse(in)
		if assert.NoError(t, err, in) {
			assert.Equal(t, want, got, in)
		}
	}

	for in, msg := range map[string]string{
		"gitlab.com/group/project":   "only github.com",
		"cli":                        "not a repository",
		"github.com/cli":             "not a repository",
		"github.com/cli/cli/tree/x":  "not a repository",
		"github.com/../etc":          "not a repository",
		"github.com/cli/cli@../../x": "not a branch",
		"github.com/cli/cli@a b":     "not a branch",
		"github.com/cli/cli@x?y=z":   "not a branch",
		"a/..":                       "not a repository",
		"a/.":                        "not a repository",
		"a/b@":                       "names no ref after @",
	} {
		_, err := Parse(in)
		assert.ErrorContains(t, err, msg, in)
	}
}

func TestRepositoryString(t *testing.T) {
	assert.Equal(t, "github.com/cli/cli", Repository{Owner: "cli", Name: "cli"}.String())
	assert.Equal(t, "github.com/cli/cli@v2", Repository{Owner: "cli", Name: "cli", Ref: "v2"}.String())
}

// entry is one member of a test tarball.
type entry struct {
	name     string
	body     string
	typeflag byte
	size     int64
}

func tarball(t *testing.T, entries ...entry) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		typeflag := e.typeflag
		if typeflag == 0 {
			typeflag = tar.TypeReg
		}
		size := int64(len(e.body))
		if e.size > 0 {
			size = e.size
		}
		h := &tar.Header{Name: e.name, Typeflag: typeflag, Mode: 0o644, Size: size, Linkname: "/etc/passwd"}
		switch typeflag {
		case tar.TypeXGlobalHeader:
			h = &tar.Header{Typeflag: typeflag, PAXRecords: map[string]string{"comment": "abc"}}
		case tar.TypeReg:
		default:
			h.Size = 0
		}
		require.NoError(t, tw.WriteHeader(h))
		if typeflag == tar.TypeReg {
			_, err := tw.Write([]byte(e.body))
			require.NoError(t, err)
		}
	}
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	return buf.Bytes()
}

// serve stands a fake GitHub up for one test and returns what the API was
// asked. Its raw host has no files, so nothing is restored.
func serve(t *testing.T, status int, body []byte) *http.Request {
	t.Helper()
	got := &http.Request{}
	github(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/raw/") {
			http.NotFound(w, r)
			return
		}
		*got = *r.Clone(r.Context())
		w.WriteHeader(status)
		_, _ = w.Write(body)
	})
	return got
}

// github points the API and the raw host at handler for one test.
func github(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	oldAPI, oldRaw := apiBase, rawBase
	apiBase, rawBase = server.URL, server.URL+"/raw"
	t.Cleanup(func() { apiBase, rawBase = oldAPI, oldRaw })
}

// routes answers each path with a status and body, and 404 for any other.
func routes(t *testing.T, answers map[string]string) *[]string {
	t.Helper()
	var asked []string
	github(t, func(w http.ResponseWriter, r *http.Request) {
		asked = append(asked, r.URL.Path)
		answer, ok := answers[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		status, body, _ := strings.Cut(answer, " ")
		code, _ := strconv.Atoi(status)
		w.WriteHeader(code)
		_, _ = w.Write([]byte(body))
	})
	return &asked
}

func TestFetchKeepsWhatZanadirReads(t *testing.T) {
	req := serve(t, http.StatusOK, tarball(t,
		entry{name: "pax_global_header", typeflag: tar.TypeXGlobalHeader},
		entry{name: "owner-repo-abc/", typeflag: tar.TypeDir},
		entry{name: "owner-repo-abc/.github/workflows/ci.yml", body: "on: push\n"},
		entry{name: "owner-repo-abc/Jenkinsfile.release", body: "sh 'make'\n"},
		entry{name: "owner-repo-abc/.renovaterc", body: "{}"},
		entry{name: "owner-repo-abc/Makefile", body: "lint:\n"},
		entry{name: "owner-repo-abc/mk/ci.mk", body: "ci:\n"},
		entry{name: "owner-repo-abc/Makefile.common", body: "common:\n"},
		entry{name: "owner-repo-abc/tox.ini", body: "[tox]\n"},
		entry{name: "owner-repo-abc/scripts/lint.sh", body: "golangci-lint run\n"},
		entry{name: "owner-repo-abc/.make/main.go", body: "package main\n"},
		entry{name: "owner-repo-abc/tools/.make/go.mod", body: "module make\n"},
		entry{name: "owner-repo-abc/cmd/main.go", body: "package main\n"},
		entry{name: "owner-repo-abc/go.mod", body: "module x\n"},
		entry{name: "owner-repo-abc/infra/main.tf", body: "resource {}\n"},
		entry{name: "owner-repo-abc/.github/renovate.json", body: `{"extends": ["config:base"]}`},
		entry{name: "owner-repo-abc/data/poems.json", body: strings.Repeat("x", headBytes+10)},
		entry{name: "owner-repo-abc/web/package.json", body: strings.Repeat(" ", headBytes) + `{"scripts": {}}`},
		entry{name: "owner-repo-abc/link", typeflag: tar.TypeSymlink},
	))

	dir, cleanup, err := Fetch(Repository{Owner: "owner", Name: "repo", Ref: "release/1.0"}, "secret")
	require.NoError(t, err)
	assert.Equal(t, "/repos/owner/repo/tarball/release/1.0", req.URL.Path)
	assert.Equal(t, "Bearer secret", req.Header.Get("Authorization"))

	read := func(rel string) string {
		data, err := os.ReadFile(filepath.Join(dir, rel))
		require.NoError(t, err, rel)
		return string(data)
	}
	assert.Equal(t, "on: push\n", read(".github/workflows/ci.yml"))
	assert.Equal(t, "sh 'make'\n", read("Jenkinsfile.release"))
	assert.Equal(t, "{}", read(".renovaterc"))
	assert.Equal(t, "lint:\n", read("Makefile"))
	assert.Equal(t, "ci:\n", read("mk/ci.mk"))
	assert.Equal(t, "common:\n", read("Makefile.common"))
	assert.Equal(t, "[tox]\n", read("tox.ini"))
	assert.Equal(t, "golangci-lint run\n", read("scripts/lint.sh"), "shell scripts are followed")
	assert.Equal(t, "package main\n", read(".make/main.go"))
	assert.Equal(t, "module make\n", read("tools/.make/go.mod"))
	assert.Empty(t, read("cmd/main.go"), "Go source outside .make is not read")
	assert.Empty(t, read("go.mod"), "only the name matters, so the content is not kept")
	assert.Empty(t, read("infra/main.tf"))
	assert.Equal(t, `{"extends": ["config:base"]}`, read(".github/renovate.json"))
	assert.Len(t, read("data/poems.json"), headBytes, "of other JSON, only the head IaC detection reads")
	assert.Len(t, read("web/package.json"), headBytes+15, "package.json scripts are followed, so it is kept whole")
	assert.DirExists(t, filepath.Join(dir, ".git"))
	_, err = os.Lstat(filepath.Join(dir, "link"))
	assert.True(t, os.IsNotExist(err), "symlinks are never written")

	cleanup()
	assert.NoDirExists(t, dir)
}

func TestFetchWithoutTokenOrRef(t *testing.T) {
	req := serve(t, http.StatusOK, tarball(t, entry{name: "o-r-1/a.yml", body: "x: 1\n"}))
	_, cleanup, err := Fetch(Repository{Owner: "o", Name: "r"}, "")
	require.NoError(t, err)
	defer cleanup()
	assert.Equal(t, "/repos/o/r/tarball", req.URL.Path)
	assert.Empty(t, req.Header.Get("Authorization"))
}

func TestFetchRefusesEntriesOutsideTheRepository(t *testing.T) {
	for _, name := range []string{"o-r-1/../../escaped.yml", "o-r-1/..", "o-r-1//etc/passwd"} {
		serve(t, http.StatusOK, tarball(t, entry{name: name, body: "x"}))
		_, _, err := Fetch(Repository{Owner: "o", Name: "r"}, "")
		assert.ErrorContains(t, err, "outside the repository", name)
	}
}

func TestFetchLimits(t *testing.T) {
	defer func(files int, file, kept int64) { maxFiles, maxFileBytes, maxKeptBytes = files, file, kept }(maxFiles, maxFileBytes, maxKeptBytes)

	maxFiles = 1
	serve(t, http.StatusOK, tarball(t, entry{name: "o-r-1/a", body: "x"}, entry{name: "o-r-1/b", body: "y"}))
	_, _, err := Fetch(Repository{Owner: "o", Name: "r"}, "")
	assert.ErrorContains(t, err, "more than 1 files")

	maxFiles, maxFileBytes, maxKeptBytes = 10, 4, 6
	serve(t, http.StatusOK, tarball(t, entry{name: "o-r-1/big.yml", body: "toolong"}, entry{name: "o-r-1/a.yml", body: "abcd"}))
	dir, cleanup, err := Fetch(Repository{Owner: "o", Name: "r"}, "")
	require.NoError(t, err)
	big, _ := os.ReadFile(filepath.Join(dir, "big.yml"))
	assert.Empty(t, big, "a file over the per-file limit becomes a placeholder")
	cleanup()

	serve(t, http.StatusOK, tarball(t, entry{name: "o-r-1/a.yml", body: "abcd"}, entry{name: "o-r-1/b.yml", body: "efgh"}))
	_, _, err = Fetch(Repository{Owner: "o", Name: "r"}, "")
	assert.ErrorContains(t, err, "exceed 6 bytes")
	assert.ErrorContains(t, err, "scan with --dir")
}

// A repository of JSON data used to fail on the limit for files zanadir
// never opens; past the budget for heads, the rest are placeholders.
func TestFetchRunsOutOfHeadsRatherThanFailing(t *testing.T) {
	defer func(heads int64) { maxHeadBytes = heads }(maxHeadBytes)
	maxHeadBytes = 5
	serve(t, http.StatusOK, tarball(t, entry{name: "o-r-1/a.json", body: "abcd"}, entry{name: "o-r-1/b.json", body: "efgh"}, entry{name: "o-r-1/c.json", body: "ijkl"}))
	dir, cleanup, err := Fetch(Repository{Owner: "o", Name: "r"}, "")
	require.NoError(t, err)
	defer cleanup()
	for name, want := range map[string]string{"a.json": "abcd", "b.json": "efgh", "c.json": ""} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		require.NoError(t, err)
		assert.Equal(t, want, string(data), name)
	}
}

func TestFetchExplainsARepositoryTooLargeToArchive(t *testing.T) {
	serve(t, http.StatusUnprocessableEntity, nil)
	_, _, err := Fetch(Repository{Owner: "o", Name: "r"}, "")
	assert.ErrorContains(t, err, "GitHub could not produce an archive of github.com/o/r (422 Unprocessable Entity), which usually means the repository is too large; clone it and scan with --dir")
}

func TestFetchExplainsATimeout(t *testing.T) {
	old := client
	client = &http.Client{Timeout: 50 * time.Millisecond}
	defer func() { client = old }()
	github(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		time.Sleep(200 * time.Millisecond)
	})
	_, _, err := Fetch(Repository{Owner: "o", Name: "r"}, "")
	assert.ErrorContains(t, err, "took longer than 50ms, which usually means the repository is too large; clone it and scan with --dir")
}

func TestFetchReportsHTTPFailures(t *testing.T) {
	for status, msg := range map[int]string{
		http.StatusNotFound:            "was not found",
		http.StatusForbidden:           "rate limit",
		http.StatusUnauthorized:        "token may be invalid",
		http.StatusInternalServerError: "500",
	} {
		serve(t, status, nil)
		_, _, err := Fetch(Repository{Owner: "o", Name: "r"}, "")
		assert.ErrorContains(t, err, msg, status)
	}
}

func TestFetchReportsABrokenArchive(t *testing.T) {
	serve(t, http.StatusOK, []byte("not gzip"))
	_, _, err := Fetch(Repository{Owner: "o", Name: "r"}, "")
	assert.ErrorContains(t, err, "reading the archive")

	full := tarball(t, entry{name: "o-r-1/a.yml", body: "x: 1\n"})
	serve(t, http.StatusOK, full[:len(full)/2])
	_, _, err = Fetch(Repository{Owner: "o", Name: "r"}, "")
	assert.Error(t, err)
}

func TestFetchReportsAnUnreachableHost(t *testing.T) {
	old := apiBase
	apiBase = "http://127.0.0.1:1"
	defer func() { apiBase = old }()
	_, _, err := Fetch(Repository{Owner: "o", Name: "r"}, "")
	assert.ErrorContains(t, err, "downloading github.com/o/r")
}

func TestFetchSkipsTheArchivesOwnGitDirectory(t *testing.T) {
	serve(t, http.StatusOK, tarball(t, entry{name: "o-r-1/.git", body: "gitdir: elsewhere"}, entry{name: "o-r-1/.git/config", body: "x"}, entry{name: "o-r-1/.GIT/refs/remotes/origin/HEAD", body: ""}))
	dir, cleanup, err := Fetch(Repository{Owner: "o", Name: "r"}, "")
	require.NoError(t, err)
	defer cleanup()
	assert.DirExists(t, filepath.Join(dir, ".git"))
	assert.NoFileExists(t, filepath.Join(dir, ".git", "config"))
	assert.NoDirExists(t, filepath.Join(dir, ".GIT", "refs"), "a case variant is skipped too, as it is .git on macOS and Windows")
}

// A file and a directory with the same name, either way round, cannot both
// be written; the archive is reported rather than half-written.
func TestFetchReportsEntriesThatCannotBeWritten(t *testing.T) {
	for _, entries := range [][]entry{
		{{name: "o-r-1/a", body: "x"}, {name: "o-r-1/a/b.yml", body: "y"}},
		{{name: "o-r-1/a", body: "x"}, {name: "o-r-1/a/", typeflag: tar.TypeDir}},
		{{name: "o-r-1/d/", typeflag: tar.TypeDir}, {name: "o-r-1/d", body: "x"}},
	} {
		serve(t, http.StatusOK, tarball(t, entries...))
		_, _, err := Fetch(Repository{Owner: "o", Name: "r"}, "")
		assert.Error(t, err, entries[1].name)
	}
}

func TestFetchReportsSetupFailures(t *testing.T) {
	t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "missing"))
	_, _, err := Fetch(Repository{Owner: "o", Name: "r"}, "")
	assert.Error(t, err, "no temporary directory")

	t.Setenv("TMPDIR", t.TempDir())
	old := apiBase
	apiBase = "://not a url"
	defer func() { apiBase = old }()
	_, _, err = Fetch(Repository{Owner: "o", Name: "r"}, "")
	assert.Error(t, err)
}

func TestToken(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "")
	assert.Empty(t, Token())
	t.Setenv("GH_TOKEN", "gh")
	assert.Equal(t, "gh", Token())
	t.Setenv("GITHUB_TOKEN", "github")
	assert.Equal(t, "github", Token(), "GITHUB_TOKEN wins, as it does in Actions")
}

func tree(paths ...string) string {
	entries := make([]string, 0, len(paths))
	for _, p := range paths {
		kind, size := "blob", "8"
		if strings.HasSuffix(p, "/") {
			kind, p = "tree", strings.TrimSuffix(p, "/")
		}
		if name, n, ok := strings.Cut(p, "#"); ok {
			p, size = name, n
		}
		entries = append(entries, fmt.Sprintf(`{"path":%q,"type":%q,"size":%s}`, p, kind, size))
	}
	return "200 " + `{"tree":[` + strings.Join(entries, ",") + "]}"
}

// git archive leaves out what .gitattributes marks export-ignore, so a
// project that marks .github that way looked like one with no CI.
func TestFetchRestoresWhatTheArchiveLeavesOut(t *testing.T) {
	archive := tarball(t,
		entry{name: "o-r-1/.gitattributes", body: "/.github export-ignore\n/tests export-ignore\n"},
		entry{name: "o-r-1/keep.yml", body: "kept: true"},
	)
	asked := routes(t, map[string]string{
		"/repos/o/r/tarball":                     "200 " + string(archive),
		"/repos/o/r/git/trees/HEAD":              tree(".github/", ".github/workflows/ci.yml", "tests/fixtures/Dockerfile", "keep.yml", "big.yml#99999999", "src/Tests/Fixtures/config.yml", "bridge/composer.json"),
		"/raw/o/r/HEAD/.github/workflows/ci.yml": "200 on: push",
	})

	dir, cleanup, err := Fetch(Repository{Owner: "o", Name: "r"}, "")
	require.NoError(t, err)
	defer cleanup()
	read := func(rel string) string {
		data, err := os.ReadFile(filepath.Join(dir, rel))
		require.NoError(t, err, rel)
		return string(data)
	}
	assert.Equal(t, "on: push", read(".github/workflows/ci.yml"))
	assert.Empty(t, read("tests/fixtures/Dockerfile"), "a file zanadir only looks at the name of is a placeholder")
	assert.Empty(t, read("big.yml"), "a file over the size limit is a placeholder")
	assert.Equal(t, "kept: true", read("keep.yml"), "a file the archive has is left alone")
	assert.NotContains(t, *asked, "/raw/o/r/HEAD/keep.yml")
	assert.Empty(t, read("src/Tests/Fixtures/config.yml"), "test fixtures are not fetched")
	assert.Empty(t, read("bridge/composer.json"), "nor JSON, of which only a head would be read")
	assert.NotContains(t, *asked, "/raw/o/r/HEAD/src/Tests/Fixtures/config.yml")
	assert.NotContains(t, *asked, "/raw/o/r/HEAD/bridge/composer.json")
}

// .gitattributes often leaves itself out, so it is fetched when missing.
func TestFetchReadsAGitattributesTheArchiveLeavesOut(t *testing.T) {
	routes(t, map[string]string{
		"/repos/o/r/tarball":                        "200 " + string(tarball(t, entry{name: "o-r-1/go.mod", body: "module x"})),
		"/raw/o/r/release/1.0/.gitattributes":       "200 .* export-ignore",
		"/repos/o/r/tarball/release/1.0":            "200 " + string(tarball(t, entry{name: "o-r-1/go.mod", body: "module x"})),
		"/repos/o/r/git/trees/release/1.0":          tree(".circleci/config.yml#12"),
		"/raw/o/r/release/1.0/.circleci/config.yml": "200 version: 2.1",
	})
	dir, cleanup, err := Fetch(Repository{Owner: "o", Name: "r", Ref: "release/1.0"}, "")
	require.NoError(t, err)
	defer cleanup()
	data, err := os.ReadFile(filepath.Join(dir, ".circleci", "config.yml"))
	require.NoError(t, err)
	assert.Equal(t, "version: 2.1", string(data))
}

// A fetched repository gets origin/HEAD, as git clone writes it.
func TestFetchRecordsTheDefaultBranch(t *testing.T) {
	routes(t, map[string]string{
		"/repos/o/r":         `200 {"default_branch": "develop"}`,
		"/repos/o/r/tarball": "200 " + string(tarball(t, entry{name: "o-r-1/go.mod", body: "module x"})),
	})
	dir, cleanup, err := Fetch(Repository{Owner: "o", Name: "r"}, "")
	require.NoError(t, err)
	defer cleanup()
	head, err := os.ReadFile(filepath.Join(dir, ".git", "refs", "remotes", "origin", "HEAD"))
	require.NoError(t, err)
	assert.Equal(t, "ref: refs/remotes/origin/develop\n", string(head))
}

func TestFetchWithNothingLeftOut(t *testing.T) {
	asked := routes(t, map[string]string{
		"/repos/o/r/tarball": "200 " + string(tarball(t, entry{name: "o-r-1/.gitattributes", body: "* text=auto"})),
	})
	_, cleanup, err := Fetch(Repository{Owner: "o", Name: "r"}, "")
	require.NoError(t, err)
	cleanup()
	assert.Equal(t, []string{"/repos/o/r", "/repos/o/r/commits/HEAD", "/repos/o/r/tarball"}, *asked, "the default branch, no file list, then no export-ignore, nothing more to fetch")
}

func TestFetchReportsFilesItCannotRestore(t *testing.T) {
	marked := "200 " + string(tarball(t, entry{name: "o-r-1/.gitattributes", body: "/.github export-ignore"}))
	for name, tt := range map[string]struct {
		answers map[string]string
		want    string
	}{
		"attributes unavailable": {map[string]string{
			"/repos/o/r/tarball":           "200 " + string(tarball(t, entry{name: "o-r-1/x", body: "y"})),
			"/raw/o/r/HEAD/.gitattributes": "500 "}, "checking for files the archive leaves out: GitHub answered 500"},
		"tree refused": {map[string]string{"/repos/o/r/tarball": marked,
			"/repos/o/r/git/trees/HEAD": "403 "}, "leaves files out of its archive (export-ignore), and fetching them failed: GitHub answered 403"},
		"tree unreadable": {map[string]string{"/repos/o/r/tarball": marked,
			"/repos/o/r/git/trees/HEAD": "200 not json"}, "fetching them failed"},
		"file missing": {map[string]string{"/repos/o/r/tarball": marked,
			"/repos/o/r/git/trees/HEAD": tree(".github/workflows/ci.yml")}, "GitHub answered 404"},
		"entry outside": {map[string]string{"/repos/o/r/tarball": marked,
			"/repos/o/r/git/trees/HEAD": tree("../escape.yml")}, "outside the repository"},
	} {
		routes(t, tt.answers)
		_, _, err := Fetch(Repository{Owner: "o", Name: "r"}, "")
		assert.ErrorContains(t, err, tt.want, name)
		if strings.Contains(tt.want, "export-ignore") {
			assert.ErrorContains(t, err, "scan a checkout with --dir", name)
		}
	}
}

func TestFetchRestoresWithinLimits(t *testing.T) {
	marked := "200 " + string(tarball(t, entry{name: "o-r-1/.gitattributes", body: "/.github export-ignore"}))
	files, kept := maxFiles, maxKeptBytes
	t.Cleanup(func() { maxFiles, maxKeptBytes = files, kept })

	routes(t, map[string]string{"/repos/o/r/tarball": marked, "/repos/o/r/git/trees/HEAD": tree("a", "b", "c")})
	maxFiles = 2
	_, _, err := Fetch(Repository{Owner: "o", Name: "r"}, "")
	assert.ErrorContains(t, err, "more than 2 files; clone it and scan with --dir")

	maxFiles = files
	routes(t, map[string]string{"/repos/o/r/tarball": marked, "/repos/o/r/git/trees/HEAD": tree("a.yml#600", "b.yml#600"),
		"/raw/o/r/HEAD/a.yml": "200 " + strings.Repeat("a", 600)})
	maxKeptBytes = 1000
	_, _, err = Fetch(Repository{Owner: "o", Name: "r"}, "")
	assert.ErrorContains(t, err, "exceed 1000 bytes")
}

func TestFetchReportsAnUnreachableRawHost(t *testing.T) {
	routes(t, map[string]string{"/repos/o/r/tarball": "200 " + string(tarball(t, entry{name: "o-r-1/x", body: "y"}))})
	rawBase = "http://127.0.0.1:1"
	_, _, err := Fetch(Repository{Owner: "o", Name: "r"}, "")
	assert.ErrorContains(t, err, "checking for files the archive leaves out")

	marked := "200 " + string(tarball(t, entry{name: "o-r-1/.gitattributes", body: "/.github export-ignore"}))
	routes(t, map[string]string{"/repos/o/r/tarball": marked, "/repos/o/r/git/trees/HEAD": tree(".github/ci.yml")})
	rawBase = "http://127.0.0.1:1"
	_, _, err = Fetch(Repository{Owner: "o", Name: "r"}, "")
	assert.ErrorContains(t, err, "fetching them failed")
}

func TestGetJSONReportsAnUnreachableHost(t *testing.T) {
	var out struct{}
	assert.Error(t, getJSON(context.Background(), "http://127.0.0.1:1/x", "", &out))
}

func TestGetJSONStopsAtTheSizeLimit(t *testing.T) {
	defer func(limit int64) { maxJSONBytes = limit }(maxJSONBytes)
	maxJSONBytes = 8
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"default_branch": "main"}`))
	}))
	defer server.Close()
	var out struct{}
	assert.Error(t, getJSON(context.Background(), server.URL, "", &out))
}

const sha = "0123456789abcdef0123456789abcdef01234567"

// fileList serves a repository through the commit, tree and raw endpoints,
// honouring Range as raw.githubusercontent.com does, and records what was
// asked with which Range.
func fileList(t *testing.T, ref, tree string, files map[string]string, extra map[string]string) *[]string {
	t.Helper()
	var mu sync.Mutex
	var asked []string
	github(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		asked = append(asked, strings.TrimSpace(r.URL.Path+" "+r.Header.Get("Range")))
		mu.Unlock()
		switch {
		case r.URL.Path == "/repos/o/r/commits/"+ref && r.Header.Get("Accept") == "application/vnd.github.sha":
			_, _ = w.Write([]byte(sha))
		case r.URL.Path == "/repos/o/r/git/trees/"+sha && r.URL.Query().Get("recursive") == "1":
			_, _ = w.Write([]byte(tree))
		case strings.HasPrefix(r.URL.Path, "/raw/o/r/"+sha+"/"):
			body, ok := files[strings.TrimPrefix(r.URL.Path, "/raw/o/r/"+sha+"/")]
			if !ok {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			var end int
			if _, err := fmt.Sscanf(r.Header.Get("Range"), "bytes=0-%d", &end); err == nil && end+1 < len(body) {
				w.WriteHeader(http.StatusPartialContent)
				body = body[:end+1]
			}
			_, _ = w.Write([]byte(body))
		default:
			answer, ok := extra[r.URL.Path]
			if !ok {
				http.NotFound(w, r)
				return
			}
			status, body, _ := strings.Cut(answer, " ")
			code, _ := strconv.Atoi(status)
			w.WriteHeader(code)
			_, _ = w.Write([]byte(body))
		}
	})
	return &asked
}

func TestFetchDownloadsOnlyWhatZanadirReads(t *testing.T) {
	stack := `{"AWSTemplateFormatVersion": "2010-09-09"}` + strings.Repeat(" ", headBytes)
	asked := fileList(t, "release/1.0", `{"tree":[
		{"path":".github","type":"tree"},
		{"path":".github/workflows/ci.yml","mode":"100644","type":"blob","size":9},
		{"path":"scripts/lint.sh","mode":"100755","type":"blob","size":18},
		{"path":"cmd/main.go","mode":"100644","type":"blob","size":13},
		{"path":"infra/stack.json","mode":"100644","type":"blob","size":`+strconv.Itoa(len(stack))+`},
		{"path":"tests/fixtures/stack.json","mode":"100644","type":"blob","size":40},
		{"path":".vscode/settings.json","mode":"100644","type":"blob","size":40},
		{"path":"big.yml","mode":"100644","type":"blob","size":99999999},
		{"path":"empty.yml","mode":"100644","type":"blob","size":0},
		{"path":"link.yml","mode":"120000","type":"blob","size":11},
		{"path":"vendor/lib","mode":"160000","type":"commit"},
		{"path":".GIT/config","mode":"100644","type":"blob","size":3}
	],"truncated":false}`, map[string]string{
		".github/workflows/ci.yml": "on: push\n",
		"scripts/lint.sh":          "golangci-lint run\n",
		"infra/stack.json":         stack,
	}, nil)

	dir, cleanup, err := Fetch(Repository{Owner: "o", Name: "r", Ref: "release/1.0"}, "")
	require.NoError(t, err)
	defer cleanup()
	read := func(rel string) string {
		data, err := os.ReadFile(filepath.Join(dir, rel))
		require.NoError(t, err, rel)
		return string(data)
	}
	assert.Equal(t, "on: push\n", read(".github/workflows/ci.yml"))
	assert.Equal(t, "golangci-lint run\n", read("scripts/lint.sh"))
	assert.Empty(t, read("cmd/main.go"), "only the name matters")
	assert.Equal(t, stack[:headBytes], read("infra/stack.json"), "only the head IaC detection reads")
	assert.Empty(t, read("tests/fixtures/stack.json"), "IaC detection skips test directories")
	assert.Empty(t, read(".vscode/settings.json"), "and hidden ones")
	assert.Empty(t, read("big.yml"), "a file over the per-file limit is a placeholder")
	assert.Empty(t, read("empty.yml"))
	for _, rel := range []string{"link.yml", "vendor/lib", ".GIT/config"} {
		_, err := os.Lstat(filepath.Join(dir, rel))
		assert.True(t, os.IsNotExist(err), rel)
	}

	assert.ElementsMatch(t, []string{
		"/repos/o/r",
		"/repos/o/r/commits/release/1.0",
		"/repos/o/r/git/trees/" + sha,
		"/raw/o/r/" + sha + "/.github/workflows/ci.yml bytes=0-8",
		"/raw/o/r/" + sha + "/scripts/lint.sh bytes=0-17",
		"/raw/o/r/" + sha + "/infra/stack.json bytes=0-" + strconv.Itoa(headBytes-1),
	}, *asked, "no archive, and every file from the one commit")
}

func TestFetchFallsBackToTheArchive(t *testing.T) {
	defer func(fetches, files int, kept int64) {
		maxFetches, maxFiles, maxKeptBytes = fetches, files, kept
	}(maxFetches, maxFiles, maxKeptBytes)
	archive := "200 " + string(tarball(t,
		entry{name: "o-r-1/.gitattributes", body: "* text=auto"},
		entry{name: "o-r-1/from-archive.yml", body: "x: 1\n"},
	))
	twoFiles := `{"tree":[{"path":"a.yml","type":"blob","size":2},{"path":"b.yml","type":"blob","size":2}]}`
	both := map[string]string{"a.yml": "a\n", "b.yml": "b\n"}

	for name, tt := range map[string]struct {
		tree  string
		files map[string]string
		setup func()
	}{
		"truncated file list":    {`{"tree":[],"truncated":true}`, nil, nil},
		"unreadable file list":   {`not json`, nil, nil},
		"too many downloads":     {twoFiles, both, func() { maxFetches = 1 }},
		"too much configuration": {`{"tree":[{"path":"a.yml","type":"blob","size":20},{"path":"b.yml","type":"blob","size":20}]}`, nil, func() { maxKeptBytes = 30 }},
		"a download that fails":  {twoFiles, map[string]string{"a.yml": "a\n"}, nil},
		"an unreachable host":    {twoFiles, both, func() { rawBase = "http://127.0.0.1:1" }},
		"an entry outside":       {`{"tree":[{"path":"../x.yml","type":"blob","size":2}]}`, nil, nil},
		"a file and a directory": {`{"tree":[{"path":"a.yml","type":"blob","size":0},{"path":"a.yml/b.go","type":"blob","size":1}]}`, nil, nil},
	} {
		maxFetches, maxFiles, maxKeptBytes = 1000, 200_000, 256<<20
		asked := fileList(t, "HEAD", tt.tree, tt.files, map[string]string{"/repos/o/r/tarball": archive})
		if tt.setup != nil {
			tt.setup()
		}
		dir, cleanup, err := Fetch(Repository{Owner: "o", Name: "r"}, "")
		require.NoError(t, err, name)
		assert.FileExists(t, filepath.Join(dir, "from-archive.yml"), name)
		assert.NoFileExists(t, filepath.Join(dir, "a.yml"), name+": nothing half-written is left")
		assert.DirExists(t, filepath.Join(dir, ".git"), name)
		assert.Contains(t, *asked, "/repos/o/r/tarball", name)
		cleanup()
	}

	maxFiles = 1
	asked := fileList(t, "HEAD", twoFiles, both, map[string]string{"/repos/o/r/tarball": archive})
	_, _, err := Fetch(Repository{Owner: "o", Name: "r"}, "")
	assert.ErrorContains(t, err, "the archive has more than 1 files", "the archive has the same limit, and reports it")
	assert.Contains(t, *asked, "/repos/o/r/tarball")
}

func TestResetTo(t *testing.T) {
	assert.Error(t, resetTo(filepath.Join(t.TempDir(), "missing")))

	dir := t.TempDir()
	locked := filepath.Join(dir, "locked")
	require.NoError(t, os.MkdirAll(filepath.Join(locked, "sub"), 0o755))
	require.NoError(t, os.Chmod(locked, 0o500))
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })
	assert.Error(t, resetTo(dir), "a directory it cannot empty")
}

func TestFetchSendsTheTokenToEveryHost(t *testing.T) {
	var mu sync.Mutex
	auth := map[string]string{}
	github(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		auth[r.URL.Path] = r.Header.Get("Authorization")
		mu.Unlock()
		switch {
		case strings.HasSuffix(r.URL.Path, "/commits/HEAD"):
			_, _ = w.Write([]byte(sha))
		case strings.Contains(r.URL.Path, "/git/trees/"):
			_, _ = w.Write([]byte(`{"tree":[{"path":"ci.yml","type":"blob","size":2}]}`))
		default:
			_, _ = w.Write([]byte("x\n"))
		}
	})
	_, cleanup, err := Fetch(Repository{Owner: "o", Name: "r"}, "secret")
	require.NoError(t, err)
	defer cleanup()
	assert.Equal(t, "Bearer secret", auth["/raw/o/r/"+sha+"/ci.yml"])
	assert.Equal(t, "Bearer secret", auth["/repos/o/r/git/trees/"+sha])
}
