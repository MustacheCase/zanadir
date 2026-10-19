package app

import (
	"bytes"
	"errors"
	"fmt"
	"os"
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
	fetchRepository = func(remote.Repository, string) (string, func(), error) {
		if err != nil {
			return "", nil, err
		}
		return "/tmp/fetched", func() { removed = true }, nil
	}
	t.Cleanup(func() { fetchRepository = remote.Fetch })
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
