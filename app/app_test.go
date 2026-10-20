package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/MustacheCase/zanadir/internal/testrepo"
	"github.com/MustacheCase/zanadir/org"
	"github.com/MustacheCase/zanadir/output"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/MustacheCase/zanadir/config"
	"github.com/MustacheCase/zanadir/models"
	"github.com/MustacheCase/zanadir/remote"
	"github.com/MustacheCase/zanadir/rules"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestNewApp(t *testing.T) {
	app := NewApp()
	assert.NotNil(t, app, "NewApp should return a non-nil root command")
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
	err := scanRepo(context.Background(), mockConfig)
	assert.NotNil(t, err, "scanRepo should return an error when handler setup fails")
}

func TestScanErrorReport(t *testing.T) {
	assert.Equal(t, "Enforcement failed: uncovered categories: SCA\n", scanErrorReport(models.NewEnforceError("uncovered categories: SCA")))
	assert.Equal(t, "Enforcement failed: execute: uncovered categories: Coverage\n",
		scanErrorReport(fmt.Errorf("execute: %w", models.NewEnforceError("uncovered categories: Coverage"))))
	assert.Equal(t, "Error: scan repo failed: handler setup failed\n", scanErrorReport(errors.New("handler setup failed")))
}

func TestFixRepo(t *testing.T) {
	err := fixRepo(context.Background(), &config.Config{})
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
		func(context.Context, *config.Config) error { did = true; return nil })(&cobra.Command{}, nil)

	assert.Equal(t, 1, *code)
	assert.False(t, did, "nothing runs without a configuration")
	assert.Equal(t, "Error: no --dir\n", diagnostics.String(), "on stderr, so a report on stdout stays parseable")
}

func TestRunExitsWhenTheWorkFails(t *testing.T) {
	code := exitCode(t)
	run(func(*cobra.Command) (*config.Config, error) { return &config.Config{}, nil },
		func(context.Context, *config.Config) error {
			return models.NewEnforceError("uncovered categories: SCA")
		})(&cobra.Command{}, nil)

	assert.Equal(t, 1, *code)
	assert.Equal(t, "Enforcement failed: uncovered categories: SCA\n", diagnostics.String())
}

func TestRunDoesNotExitOnSuccess(t *testing.T) {
	code := exitCode(t)
	run(func(*cobra.Command) (*config.Config, error) { return &config.Config{}, nil },
		func(context.Context, *config.Config) error { return nil })(&cobra.Command{}, nil)

	assert.Equal(t, -1, *code)
}

func TestABadCustomRuleStopsBothCommands(t *testing.T) {
	cfg := &config.Config{Rules: []rules.FileRule{{ID: "no-regex"}}}
	assert.ErrorContains(t, scanRepo(context.Background(), cfg), "no-regex")
	assert.ErrorContains(t, fixRepo(context.Background(), cfg), "no-regex")
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
	run(repoConfig, func(_ context.Context, cfg *config.Config) error { scanned = cfg.Dir; return nil })(&cobra.Command{}, nil)

	assert.Equal(t, "/tmp/fetched", scanned)
	assert.True(t, *removed)
	assert.Equal(t, -1, *code)
}

func TestRunRemovesTheFetchedRepositoryWhenTheScanFails(t *testing.T) {
	code := exitCode(t)
	removed := fakeFetch(t, nil)
	run(repoConfig, func(context.Context, *config.Config) error { return errors.New("boom") })(&cobra.Command{}, nil)

	assert.True(t, *removed, "exit skips deferred calls, so cleanup must not be deferred")
	assert.Equal(t, 1, *code)
}

func TestRunExitsWhenTheFetchFails(t *testing.T) {
	code := exitCode(t)
	fakeFetch(t, errors.New("github.com/o/r was not found"))
	did := false
	run(repoConfig, func(context.Context, *config.Config) error { did = true; return nil })(&cobra.Command{}, nil)

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
	run(repoConfig, func(context.Context, *config.Config) error { scanned = true; return nil })(&cobra.Command{}, nil)

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
	run(repoConfig, func(context.Context, *config.Config) error {
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

func fakeOrganisation(t *testing.T, scan func(context.Context, org.Options) (output.OrgReport, error)) {
	t.Helper()
	scanOrganisation = scan
	t.Cleanup(func() { scanOrganisation = org.Scan })
}

func TestScanOrgNeedsAToken(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "")
	assert.ErrorContains(t, scanRepo(context.Background(), &config.Config{Org: "acme"}), "--org needs GITHUB_TOKEN")
}

// Each repository is assessed as a --repo scan of its fetched copy.
func TestScanOrgAssessesEachRepositoryAndWritesTheReport(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "t")
	dir := testrepo.Write(t, map[string]string{".git/HEAD": "ref: refs/heads/main\n", ".github/workflows/ci.yml": "on: pull_request\njobs:\n  a:\n    runs-on: x\n    steps:\n      - run: gitleaks detect\n"})
	fakeOrganisation(t, func(_ context.Context, opts org.Options) (output.OrgReport, error) {
		assert.Equal(t, "acme", opts.Org)
		assert.Equal(t, "t", opts.Token)
		report, err := opts.Assess(remote.Repository{Owner: "acme", Name: "web"}, dir)
		require.NoError(t, err)
		return output.OrgReport{Org: "acme", Repositories: []output.OrgRepository{{Name: "acme/web", Report: report}}}, nil
	})
	out := filepath.Join(t.TempDir(), "org.json")
	require.NoError(t, scanRepo(context.Background(), &config.Config{Org: "acme", Parallel: 2, Output: config.OutputJSON, OutputFile: out}))
	data, err := os.ReadFile(out)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"repository": "acme/web"`)
	assert.NotContains(t, string(data), `"id": "Secrets Detection"`, "the repository's own workflow was read: gitleaks covers it")
}

func TestScanOrgFailsWhenNothingCouldBeScanned(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "t")
	fakeOrganisation(t, func(context.Context, org.Options) (output.OrgReport, error) {
		return output.OrgReport{Org: "acme", Failed: []output.OrgRepository{{Name: "acme/a", Reason: "x"}}}, nil
	})
	assert.ErrorContains(t, scanRepo(context.Background(), &config.Config{Org: "acme", OutputFile: filepath.Join(t.TempDir(), "r.txt")}), "none of the 1 repositories of acme could be scanned")

	fakeOrganisation(t, func(context.Context, org.Options) (output.OrgReport, error) {
		return output.OrgReport{}, errors.New("listing failed")
	})
	assert.ErrorContains(t, scanRepo(context.Background(), &config.Config{Org: "acme"}), "listing failed")

	fakeOrganisation(t, func(context.Context, org.Options) (output.OrgReport, error) {
		return output.OrgReport{Org: "acme"}, nil
	})
	assert.Error(t, scanRepo(context.Background(), &config.Config{Org: "acme", OutputFile: filepath.Join(t.TempDir(), "no", "r.txt")}))
}

// An organisation scan stops on the signal itself, so the exit waits for it
// to finish writing what it has.
func TestRunWaitsForAnOrganisationScanOnASignal(t *testing.T) {
	code := exitCode(t)
	signals := fakeSignals(t)
	finished := false
	run(func(*cobra.Command) (*config.Config, error) { return &config.Config{Org: "acme"}, nil },
		func(ctx context.Context, _ *config.Config) error {
			signals <- syscall.SIGTERM
			<-ctx.Done()
			finished = true
			return nil
		})(&cobra.Command{}, nil)

	assert.True(t, finished)
	assert.Equal(t, 128+15, *code)
}
