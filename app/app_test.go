package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"syscall"
	"testing"

	"github.com/MustacheCase/zanadir/config"
	"github.com/MustacheCase/zanadir/mcpserver"
	"github.com/MustacheCase/zanadir/models"
	"github.com/MustacheCase/zanadir/remote"
	"github.com/MustacheCase/zanadir/rules"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestNewApp(t *testing.T) {
	app := NewApp()
	assert.NotNil(t, app, "NewApp should return a non-nil root command")
	assert.Equal(t, Version(), app.Version, "cobra prints it for --version")
}

func TestScanCmdFlags(t *testing.T) {
	cmd := scanCmd

	dirFlag := cmd.Flags().Lookup("dir")
	assert.NotNil(t, dirFlag, "The 'dir' flag should be defined")
	assert.Equal(t, "Path to the repository directory (required unless --repo is given)", dirFlag.Usage)

	excludedCategoriesFlag := cmd.Flags().Lookup("excluded-categories")
	assert.NotNil(t, excludedCategoriesFlag, "The 'excluded-categories' flag should be defined")
	assert.Equal(t, "List of excluded categories (optional)", excludedCategoriesFlag.Usage)

	enforceFlag := cmd.Flags().Lookup("enforce")
	assert.NotNil(t, enforceFlag, "The 'enforce' flag should be defined")
	assert.Equal(t, "Fails the CI process when any category is uncovered (optional)", enforceFlag.Usage)

	assert.NotNil(t, cmd.Flags().Lookup("fail-on"), "The 'fail-on' flag should be defined")
	assert.NotNil(t, cmd.Flags().Lookup("baseline"), "The 'baseline' flag should be defined")
	assert.NotNil(t, cmd.Flags().Lookup("write-baseline"), "The 'write-baseline' flag should be defined")
}

func TestScanRepo(t *testing.T) {
	mockConfig := &config.Config{}
	err := scanRepo(mockConfig)
	assert.NotNil(t, err, "scanRepo should return an error when handler setup fails")
}

func TestScanErrorReport(t *testing.T) {
	assert.Equal(t, "Enforcement failed: uncovered categories: SCA\n", scanErrorReport(models.NewEnforceError("uncovered categories: SCA")))
	assert.Equal(t, "Enforcement failed: execute: uncovered categories: Coverage\n",
		scanErrorReport(fmt.Errorf("execute: %w", models.NewEnforceError("uncovered categories: Coverage"))))
	assert.Equal(t, "Error: scan repo failed: handler setup failed\n", scanErrorReport(errors.New("handler setup failed")))
}

func TestFixRepo(t *testing.T) {
	err := fixRepo(&config.Config{})
	assert.NotNil(t, err, "fixRepo should return an error when the directory is not a repository")
}

// diagnostics holds what the last run wrote to stderr.
var diagnostics bytes.Buffer

// exitCode records what run would have exited with, -1 for not at all, and
// captures its stderr in diagnostics.
func exitCode(t *testing.T) *int {
	t.Helper()
	code := -1
	exit = func(c int) { code = c }
	diagnostics.Reset()
	stderr = &diagnostics
	t.Cleanup(func() { exit, stderr = os.Exit, os.Stderr })
	return &code
}

func TestRunExitsWhenTheConfigurationIsBad(t *testing.T) {
	code := exitCode(t)
	did := false
	run(func(*cobra.Command) (*config.Config, error) { return nil, errors.New("no --dir") },
		func(*config.Config) error { did = true; return nil })(&cobra.Command{}, nil)

	assert.Equal(t, 1, *code)
	assert.False(t, did, "nothing runs without a configuration")
	assert.Equal(t, "Error: no --dir\n", diagnostics.String(), "on stderr, so a report on stdout stays parseable")
}

func TestRunExitsWhenTheWorkFails(t *testing.T) {
	code := exitCode(t)
	run(func(*cobra.Command) (*config.Config, error) { return &config.Config{}, nil },
		func(*config.Config) error { return models.NewEnforceError("uncovered categories: SCA") })(&cobra.Command{}, nil)

	assert.Equal(t, 1, *code)
	assert.Equal(t, "Enforcement failed: uncovered categories: SCA\n", diagnostics.String())
}

func TestRunDoesNotExitOnSuccess(t *testing.T) {
	code := exitCode(t)
	run(func(*cobra.Command) (*config.Config, error) { return &config.Config{}, nil },
		func(*config.Config) error { return nil })(&cobra.Command{}, nil)

	assert.Equal(t, -1, *code)
}

func TestABadCustomRuleStopsBothCommands(t *testing.T) {
	cfg := &config.Config{Rules: []rules.FileRule{{ID: "no-regex"}}}
	assert.ErrorContains(t, scanRepo(cfg), "no-regex")
	assert.ErrorContains(t, fixRepo(cfg), "no-regex")
}

// fakeFetch stands in for remote.Fetch and records whether the copy it
// handed out was removed.
func fakeFetch(t *testing.T, err error) *bool {
	t.Helper()
	removed := false
	fetchRepository = func(context.Context, remote.Repository, string) (string, func(), error) {
		if err != nil {
			return "", nil, err
		}
		return "/tmp/fetched", func() { removed = true }, nil
	}
	t.Cleanup(func() { fetchRepository = remote.FetchContext })
	return &removed
}

func repoConfig(*cobra.Command) (*config.Config, error) {
	return &config.Config{Repo: &remote.Repository{Owner: "o", Name: "r"}}, nil
}

func TestRunScansAFetchedRepositoryAndRemovesIt(t *testing.T) {
	code := exitCode(t)
	removed := fakeFetch(t, nil)
	var scanned string
	run(repoConfig, func(cfg *config.Config) error { scanned = cfg.Dir; return nil })(&cobra.Command{}, nil)

	assert.Equal(t, "/tmp/fetched", scanned)
	assert.True(t, *removed)
	assert.Equal(t, -1, *code)
}

func TestRunRemovesTheFetchedRepositoryWhenTheScanFails(t *testing.T) {
	code := exitCode(t)
	removed := fakeFetch(t, nil)
	run(repoConfig, func(*config.Config) error { return errors.New("boom") })(&cobra.Command{}, nil)

	assert.True(t, *removed, "exit skips deferred calls, so cleanup must not be deferred")
	assert.Equal(t, 1, *code)
}

func TestRunExitsWhenTheFetchFails(t *testing.T) {
	code := exitCode(t)
	fakeFetch(t, errors.New("github.com/o/r was not found"))
	did := false
	run(repoConfig, func(*config.Config) error { did = true; return nil })(&cobra.Command{}, nil)

	assert.Equal(t, 1, *code)
	assert.False(t, did)
	assert.Equal(t, "Error: github.com/o/r was not found\n", diagnostics.String())
}

// fakeSignals stands in for the process's signals for one test.
func fakeSignals(t *testing.T) chan os.Signal {
	t.Helper()
	signals := make(chan os.Signal, 1)
	interrupts = func() (<-chan os.Signal, func()) { return signals, func() {} }
	t.Cleanup(func() { interrupts = defaultInterrupts })
	return signals
}

// A cancelled CI job sends SIGTERM while the repository downloads: the
// download is abandoned, and zanadir exits as the signal would have.
func TestRunAbandonsTheDownloadOnASignal(t *testing.T) {
	code := exitCode(t)
	signals := fakeSignals(t)
	abandoned := false
	fetchRepository = func(ctx context.Context, _ remote.Repository, _ string) (string, func(), error) {
		signals <- syscall.SIGTERM
		<-ctx.Done()
		abandoned = true
		return "", nil, ctx.Err()
	}
	t.Cleanup(func() { fetchRepository = remote.FetchContext })
	scanned := false
	run(repoConfig, func(*config.Config) error { scanned = true; return nil })(&cobra.Command{}, nil)

	assert.True(t, abandoned)
	assert.False(t, scanned)
	assert.Equal(t, 128+15, *code)
	assert.Empty(t, diagnostics.String(), "a cancelled download is not reported as an error")
}

// A signal during the scan still removes the fetched copy.
func TestRunRemovesTheFetchedRepositoryOnASignal(t *testing.T) {
	code := exitCode(t)
	signals := fakeSignals(t)
	removed := make(chan struct{})
	fetchRepository = func(context.Context, remote.Repository, string) (string, func(), error) {
		return "/tmp/fetched", func() { close(removed) }, nil
	}
	t.Cleanup(func() { fetchRepository = remote.FetchContext })
	run(repoConfig, func(*config.Config) error {
		signals <- os.Interrupt
		<-removed
		return errors.New("interrupted")
	})(&cobra.Command{}, nil)

	assert.Equal(t, 128+2, *code)
	assert.Empty(t, diagnostics.String())
}

type otherSignal struct{}

func (otherSignal) String() string { return "other" }
func (otherSignal) Signal()        {}

func TestSignalNumber(t *testing.T) {
	assert.Equal(t, 15, signalNumber(syscall.SIGTERM))
	assert.Equal(t, 1, signalNumber(otherSignal{}), "a signal that is not a number exits 128 + 1")
}

// A stray word, as in --enforce false, used to be ignored, and the scan
// enforced.
func TestScanAndFixRejectArguments(t *testing.T) {
	for _, cmd := range []*cobra.Command{scanCmd, fixCmd} {
		assert.Error(t, cmd.Args(cmd, []string{"false"}), cmd.Use)
		assert.Error(t, cmd.Args(cmd, []string{"", "false"}), cmd.Use)
		assert.NoError(t, cmd.Args(cmd, nil), cmd.Use)
		// The zanadir action passes each unset optional input as "".
		assert.NoError(t, cmd.Args(cmd, []string{"", ""}), cmd.Use)
	}
}

func TestMCPCommandServesWithTheVersion(t *testing.T) {
	old := serveMCP
	t.Cleanup(func() { serveMCP = old })
	var served mcpserver.Options
	serveMCP = func(_ context.Context, opts mcpserver.Options) error { served = opts; return nil }
	assert.NoError(t, mcpCmd.Flags().Set("dir", "/repo"))
	assert.NoError(t, mcpCmd.RunE(mcpCmd, nil))
	assert.Equal(t, mcpserver.Options{Version: Version(), Dir: "/repo"}, served)
	assert.NoError(t, mcpCmd.Args(mcpCmd, []string{""}), "an MCP client may pass empty arguments too")
	assert.Error(t, mcpCmd.Args(mcpCmd, []string{"scan"}))
}

func TestCheckSkills(t *testing.T) {
	dir := t.TempDir()
	assert.NoError(t, os.MkdirAll(dir+"/skills/a", 0o755))
	assert.NoError(t, os.WriteFile(dir+"/skills/a/SKILL.md", []byte("---\nname: a\ndescription: Does a. Use when the user asks for a.\n---\n[x](gone.md)\n"), 0o644))
	var out bytes.Buffer
	stdout = &out
	t.Cleanup(func() { stdout = os.Stdout })

	assert.NoError(t, checkSkills(&config.Config{Dir: dir, Output: config.OutputTable}))
	assert.Contains(t, out.String(), "links gone.md, which does not exist")

	out.Reset()
	err := checkSkills(&config.Config{Dir: dir, Output: config.OutputJSON, Enforce: true})
	var enforceErr *models.EnforceError
	assert.ErrorAs(t, err, &enforceErr)
	assert.Contains(t, out.String(), `"rule": "missing-reference"`)

	assert.Error(t, checkSkills(&config.Config{Dir: dir + "/absent"}))
}

func TestSkillsCommandIsRegistered(t *testing.T) {
	cmd, _, err := rootCmd.Find([]string{"skills"})
	assert.NoError(t, err)
	assert.Equal(t, "skills", cmd.Name())
	assert.NotNil(t, cmd.Flags().Lookup("enforce"))
}
