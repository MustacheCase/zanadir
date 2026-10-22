package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/MustacheCase/zanadir/handler"
	"github.com/MustacheCase/zanadir/internal/testrepo"
	"github.com/MustacheCase/zanadir/remote"
	"github.com/MustacheCase/zanadir/rules"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const workflow = "on: pull_request\njobs:\n  test:\n    runs-on: x\n    steps:\n      - run: go test ./...\n      - run: gitleaks detect || true\n"

func repo(t *testing.T) string {
	return testrepo.Write(t, map[string]string{".git/HEAD": "ref: refs/heads/main\n", ".github/workflows/ci.yml": workflow, "go.mod": "module x\n"})
}

func connect(t *testing.T) *mcp.ClientSession {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	serverEnd, clientEnd := mcp.NewInMemoryTransports()
	go func() { _ = New("1.2.3").Run(ctx, serverEnd) }()
	session, err := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil).Connect(ctx, clientEnd, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func call(t *testing.T, session *mcp.ClientSession, tool string, args map[string]any) (string, bool) {
	t.Helper()
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: tool, Arguments: args})
	require.NoError(t, err)
	require.Len(t, result.Content, 1)
	return result.Content[0].(*mcp.TextContent).Text, result.IsError
}

func TestTheServerOffersReadOnlyScanAndFix(t *testing.T) {
	session := connect(t)
	assert.Equal(t, "1.2.3", session.InitializeResult().ServerInfo.Version)
	tools, err := session.ListTools(context.Background(), nil)
	require.NoError(t, err)
	require.Len(t, tools.Tools, 2)
	for _, tool := range tools.Tools {
		assert.True(t, tool.Annotations.ReadOnlyHint, tool.Name)
		assert.Contains(t, tool.InputSchema.(map[string]any)["properties"], "repo")
	}
}

func TestScanReturnsTheJSONReport(t *testing.T) {
	text, isError := call(t, connect(t), "scan", map[string]any{"dir": repo(t), "excludedCategories": []string{"performance testing"}})
	require.False(t, isError, text)
	var report struct {
		Score     struct{ Covered, Total int }
		Platforms []string
		Weakened  []struct{ Category, Verdict string }
		Uncovered []struct{ ID string }
	}
	require.NoError(t, json.Unmarshal([]byte(text), &report))
	assert.Equal(t, []string{"github"}, report.Platforms)
	assert.Equal(t, []struct{ Category, Verdict string }{{"Secrets Detection", "advisory"}}, report.Weakened)
	assert.NotContains(t, report.Uncovered, struct{ ID string }{"Performance Testing"}, "excluded")
}

func TestFixPrintsSnippetsWithoutWriting(t *testing.T) {
	dir := repo(t)
	text, isError := call(t, connect(t), "fix", map[string]any{"dir": dir})
	require.False(t, isError, text)
	assert.Contains(t, text, "is not covered. Add to .github/workflows/")
	assert.NoFileExists(t, dir+"/.github/workflows/zanadir-suggested.yml")
}

func TestARepositoryIsFetchedAndRemoved(t *testing.T) {
	dir, removed := repo(t), false
	oldFetch := fetch
	t.Cleanup(func() { fetch = oldFetch })
	fetch = func(_ context.Context, r remote.Repository, _ string) (string, func(), error) {
		if r.Name == "gone" {
			return "", nil, errors.New("github.com/o/gone was not found")
		}
		assert.Equal(t, remote.Repository{Owner: "o", Name: "r", Ref: "v1"}, r)
		return dir, func() { removed = true }, nil
	}
	session := connect(t)

	text, isError := call(t, session, "scan", map[string]any{"repo": "github.com/o/r@v1"})
	assert.False(t, isError, text)
	assert.True(t, removed)

	for message, args := range map[string]map[string]any{
		"was not found":           {"repo": "github.com/o/gone"},
		"not both":                {"repo": "github.com/o/r", "dir": "."},
		"only github.com":         {"repo": "gitlab.com/o/r"},
		"not a git repository":    {"dir": t.TempDir()},
		`unknown category "nope"`: {"excludedCategories": []string{"nope"}},
	} {
		text, isError := call(t, session, "scan", args)
		assert.True(t, isError, message)
		assert.Contains(t, text, message)
	}
}

func TestToolsReportAFailedSetup(t *testing.T) {
	oldSetup := setup
	t.Cleanup(func() { setup = oldSetup })
	setup = func(...rules.FileRule) (*handler.Handler, error) { return nil, errors.New("no catalogue") }
	text, isError := call(t, connect(t), "fix", nil)
	assert.True(t, isError)
	assert.Contains(t, text, "no catalogue")
}

func TestRunServesOnTheTransport(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	serverEnd, clientEnd := mcp.NewInMemoryTransports()
	oldStdio := stdio
	t.Cleanup(func() { stdio = oldStdio })
	stdio = serverEnd
	done := make(chan error, 1)
	go func() { done <- Run(ctx, "9.9.9") }()
	session, err := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil).Connect(ctx, clientEnd, nil)
	require.NoError(t, err)
	assert.Equal(t, "9.9.9", session.InitializeResult().ServerInfo.Version)
	cancel()
	_ = session.Close()
	assert.Error(t, <-done, "a cancelled serve returns why it stopped")
}
