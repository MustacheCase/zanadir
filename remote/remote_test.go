package remote

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

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

// serve stands a fake GitHub up for one test and returns what it was asked.
func serve(t *testing.T, status int, body []byte) *http.Request {
	t.Helper()
	got := &http.Request{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*got = *r.Clone(r.Context())
		w.WriteHeader(status)
		_, _ = w.Write(body)
	}))
	t.Cleanup(server.Close)
	old := apiBase
	apiBase = server.URL
	t.Cleanup(func() { apiBase = old })
	return got
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
		entry{name: "owner-repo-abc/tox.ini", body: "[tox]\n"},
		entry{name: "owner-repo-abc/go.mod", body: "module x\n"},
		entry{name: "owner-repo-abc/infra/main.tf", body: "resource {}\n"},
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
	assert.Equal(t, "[tox]\n", read("tox.ini"))
	assert.Empty(t, read("go.mod"), "only the name matters, so the content is not kept")
	assert.Empty(t, read("infra/main.tf"))
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
	serve(t, http.StatusOK, tarball(t, entry{name: "o-r-1/.git", body: "gitdir: elsewhere"}, entry{name: "o-r-1/.git/config", body: "x"}))
	dir, cleanup, err := Fetch(Repository{Owner: "o", Name: "r"}, "")
	require.NoError(t, err)
	defer cleanup()
	assert.DirExists(t, filepath.Join(dir, ".git"))
	assert.NoFileExists(t, filepath.Join(dir, ".git", "config"))
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
