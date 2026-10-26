package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MustacheCase/zanadir/handler"
	"github.com/MustacheCase/zanadir/internal/testrepo"
	"github.com/MustacheCase/zanadir/remote"
	"github.com/MustacheCase/zanadir/rules"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const workflow = "on: pull_request\njobs:\n  test:\n    runs-on: x\n    steps:\n      - run: go test ./...\n      - run: gitleaks detect || true\n"

func repo(t *testing.T) string {
	return testrepo.Write(t, map[string]string{".git/HEAD": "ref: refs/heads/main\n", ".github/workflows/ci.yml": workflow, "go.mod": "module x\n"})
}

type reply struct {
	ID     int `json:"id"`
	Result struct {
		ProtocolVersion string            `json:"protocolVersion"`
		ServerInfo      map[string]string `json:"serverInfo"`
		Tools           []struct {
			Name        string          `json:"name"`
			InputSchema map[string]any  `json:"inputSchema"`
			Annotations map[string]bool `json:"annotations"`
		} `json:"tools"`
		Content []struct{ Type, Text string } `json:"content"`
		IsError bool                          `json:"isError"`
	} `json:"result"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// options are what session serves with; a test may change them.
var options = Options{Version: "1.2.3"}

// session sends messages, one per line, and returns the replies in order.
func session(t *testing.T, messages ...any) []reply {
	t.Helper()
	var in bytes.Buffer
	for _, m := range messages {
		if s, ok := m.(string); ok {
			in.WriteString(s + "\n")
			continue
		}
		require.NoError(t, json.NewEncoder(&in).Encode(m))
	}
	var out bytes.Buffer
	require.NoError(t, Serve(context.Background(), &in, &out, options))
	var replies []reply
	decoder := json.NewDecoder(&out)
	for decoder.More() {
		var r reply
		require.NoError(t, decoder.Decode(&r))
		replies = append(replies, r)
	}
	return replies
}

// serveFrom makes dir the server's --dir for one test.
func serveFrom(t *testing.T, dir string) {
	old := options
	t.Cleanup(func() { options = old })
	options.Dir = dir
}

func callMessage(id int, tool string, args any) map[string]any {
	return map[string]any{"jsonrpc": "2.0", "id": id, "method": "tools/call", "params": map[string]any{"name": tool, "arguments": args}}
}

func TestHandshakeAndToolList(t *testing.T) {
	replies := session(t,
		map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{"protocolVersion": "2025-03-26"}},
		map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"},
		"",
		map[string]any{"jsonrpc": "2.0", "id": 2, "method": "ping"},
		map[string]any{"jsonrpc": "2.0", "id": 3, "method": "tools/list"},
		map[string]any{"jsonrpc": "2.0", "id": 4, "method": "initialize", "params": map[string]any{"protocolVersion": "2099-01-01"}},
		map[string]any{"jsonrpc": "2.0", "id": 5, "method": "resources/list"},
		"{not json",
	)
	require.Len(t, replies, 6, "notifications and blank lines get no reply")
	assert.Equal(t, "2025-03-26", replies[0].Result.ProtocolVersion, "a version both speak is kept")
	assert.Equal(t, map[string]string{"name": "zanadir", "title": "zanadir", "version": "1.2.3"}, replies[0].Result.ServerInfo)
	assert.Nil(t, replies[1].Error)
	require.Len(t, replies[2].Result.Tools, 2)
	for _, tool := range replies[2].Result.Tools {
		assert.True(t, tool.Annotations["readOnlyHint"], tool.Name)
		assert.Contains(t, tool.InputSchema["properties"], "repo")
	}
	assert.Equal(t, "2025-06-18", replies[3].Result.ProtocolVersion, "an unknown version gets the newest")
	assert.Equal(t, -32601, replies[4].Error.Code)
	assert.Equal(t, -32700, replies[5].Error.Code)
}

func TestScanReturnsTheJSONReport(t *testing.T) {
	dir := repo(t)
	serveFrom(t, dir)
	replies := session(t, callMessage(1, "scan", map[string]any{"dir": dir, "excludedCategories": []string{"performance testing"}}))
	require.False(t, replies[0].Result.IsError, replies[0].Result.Content)
	var report struct {
		Platforms []string
		Weakened  []struct{ Category, Verdict string }
		Uncovered []struct{ ID string }
	}
	require.NoError(t, json.Unmarshal([]byte(replies[0].Result.Content[0].Text), &report))
	assert.Equal(t, []string{"github"}, report.Platforms)
	assert.Equal(t, []struct{ Category, Verdict string }{{"Secrets Detection", "advisory"}}, report.Weakened)
	assert.NotContains(t, report.Uncovered, struct{ ID string }{"Performance Testing"}, "excluded")
}

func TestToolsReadTheDefaultDirectory(t *testing.T) {
	old := options
	t.Cleanup(func() { options = old })
	options.Dir = repo(t)
	replies := session(t, callMessage(1, "scan", nil))
	require.False(t, replies[0].Result.IsError, replies[0].Result.Content)
	assert.Contains(t, replies[0].Result.Content[0].Text, `"github"`, "the server's directory, not the working one")
}

func TestFixPrintsSnippetsWithoutWriting(t *testing.T) {
	dir := repo(t)
	serveFrom(t, dir)
	replies := session(t, callMessage(1, "fix", map[string]any{"dir": dir}))
	require.False(t, replies[0].Result.IsError)
	assert.Contains(t, replies[0].Result.Content[0].Text, "is not covered. Add to .github/workflows/")
	assert.NoFileExists(t, dir+"/.github/workflows/zanadir-suggested.yml")
}

func TestARepositoryIsFetchedAndRemoved(t *testing.T) {
	dir, removed := repo(t), false
	base := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(base, "plain"), 0o755))
	serveFrom(t, base)
	oldFetch := fetch
	t.Cleanup(func() { fetch = oldFetch })
	fetch = func(_ context.Context, r remote.Repository, _ string) (string, func(), error) {
		if r.Name == "gone" {
			return "", nil, errors.New("github.com/o/gone was not found")
		}
		assert.Equal(t, remote.Repository{Owner: "o", Name: "r", Ref: "v1"}, r)
		return dir, func() { removed = true }, nil
	}
	replies := session(t, callMessage(1, "scan", map[string]any{"repo": "github.com/o/r@v1"}))
	assert.False(t, replies[0].Result.IsError, replies[0].Result.Content)
	assert.True(t, removed)

	for message, args := range map[string]any{
		"was not found":           map[string]any{"repo": "github.com/o/gone"},
		"not both":                map[string]any{"repo": "github.com/o/r", "dir": "."},
		"only github.com":         map[string]any{"repo": "gitlab.com/o/r"},
		`unknown category "nope"`: map[string]any{"excludedCategories": []string{"nope"}},
		"not a git repository":    map[string]any{"dir": "plain"},
		`unknown field "branch"`:  map[string]any{"branch": "main"},
		"invalid arguments":       []string{"dir"},
	} {
		replies := session(t, callMessage(1, "scan", args))
		assert.True(t, replies[0].Result.IsError, message)
		assert.Contains(t, replies[0].Result.Content[0].Text, message)
	}
}

func TestToolCallErrors(t *testing.T) {
	oldSetup := setup
	t.Cleanup(func() { setup = oldSetup })
	setup = func(...rules.FileRule) (*handler.Handler, error) { return nil, errors.New("no catalogue") }
	replies := session(t,
		callMessage(1, "fix", nil),
		callMessage(2, "deploy", nil),
		map[string]any{"jsonrpc": "2.0", "id": 3, "method": "tools/call", "params": "scan"},
	)
	assert.True(t, replies[0].Result.IsError)
	assert.Equal(t, "no catalogue", replies[0].Result.Content[0].Text)
	assert.Equal(t, -32602, replies[1].Error.Code)
	assert.Contains(t, replies[1].Error.Message, `unknown tool "deploy"`)
	assert.Equal(t, -32602, replies[2].Error.Code)
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestServeStops(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	reader, writer := io.Pipe()
	defer func() { _ = writer.Close() }()
	assert.ErrorIs(t, Serve(ctx, reader, io.Discard, Options{}), context.Canceled, "when its context ends")

	in := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}` + "\n")
	assert.ErrorIs(t, Serve(context.Background(), in, failingWriter{}, Options{}), io.ErrClosedPipe, "when it cannot reply")

	assert.Error(t, Serve(context.Background(), strings.NewReader(strings.Repeat("x", 17<<20)), io.Discard, Options{}), "on a line too long to read")
}

type gatedReader struct {
	gate <-chan struct{}
	r    io.Reader
}

func (g gatedReader) Read(p []byte) (int, error) {
	<-g.gate
	return g.r.Read(p)
}

// A line read after Serve has returned is dropped, and the reader stops.
func TestTheReaderStopsWithItsContext(t *testing.T) {
	exited := make(chan struct{})
	old := readerExited
	t.Cleanup(func() { readerExited = old })
	readerExited = func() { close(exited) }
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	gate := make(chan struct{})
	in := gatedReader{gate: gate, r: strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}` + "\n")}
	assert.ErrorIs(t, Serve(ctx, in, io.Discard, Options{}), context.Canceled)
	close(gate)
	<-exited
}

func TestRunServesStdin(t *testing.T) {
	old := stdin
	t.Cleanup(func() { stdin = old })
	stdin = strings.NewReader("")
	assert.NoError(t, Run(context.Background(), Options{}))
}

// The MCP Registry lists the image only if its label matches server.json.
func TestTheRegistryNameMatchesTheImageLabel(t *testing.T) {
	data, err := os.ReadFile("../server.json")
	require.NoError(t, err)
	var server struct{ Name string }
	require.NoError(t, json.Unmarshal(data, &server))
	dockerfile, err := os.ReadFile("../Dockerfile")
	require.NoError(t, err)
	assert.Contains(t, string(dockerfile), `LABEL io.modelcontextprotocol.server.name="`+server.Name+`"`)
	assert.Equal(t, "io.github.MustacheCase/zanadir", server.Name, "GitHub OIDC grants the namespace of the repository's owner, as GitHub spells it")
}

func TestADirStaysInsideTheServersDirectory(t *testing.T) {
	base, outside := repo(t), repo(t)
	require.NoError(t, os.Symlink(outside, filepath.Join(base, "link")))
	require.NoError(t, os.MkdirAll(filepath.Join(base, "sub", ".github"), 0o755))
	serveFrom(t, base)

	for _, dir := range []string{outside, "../" + filepath.Base(outside), "link"} {
		replies := session(t, callMessage(1, "scan", map[string]any{"dir": dir}))
		assert.True(t, replies[0].Result.IsError, dir)
		assert.Contains(t, replies[0].Result.Content[0].Text, "start zanadir mcp with --dir to read elsewhere", dir)
	}
	replies := session(t, callMessage(1, "scan", map[string]any{"dir": "missing"}))
	assert.True(t, replies[0].Result.IsError)
	for _, dir := range []string{base, ".", "sub"} {
		replies := session(t, callMessage(1, "scan", map[string]any{"dir": dir}))
		assert.NotContains(t, replies[0].Result.Content[0].Text, "outside", dir)
	}
}

func TestRepoFetchesUseTheTokenOnlyWhenAllowed(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "secret")
	dir, used := repo(t), ""
	oldFetch := fetch
	t.Cleanup(func() { fetch = oldFetch })
	fetch = func(_ context.Context, _ remote.Repository, token string) (string, func(), error) {
		used = token
		return dir, func() {}, nil
	}
	session(t, callMessage(1, "scan", map[string]any{"repo": "github.com/o/r"}))
	assert.Empty(t, used, "anonymous unless the server allows the token")

	old := options
	t.Cleanup(func() { options = old })
	options.RepoToken = true
	session(t, callMessage(1, "scan", map[string]any{"repo": "github.com/o/r"}))
	assert.Equal(t, "secret", used)
}
