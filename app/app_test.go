package app

import (
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/MustacheCase/zanadir/config"
	"github.com/MustacheCase/zanadir/models"
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
	assert.Equal(t, "Path to the repository directory (required)", dirFlag.Usage)

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

func TestScanErrorReportRoutesEnforcementToStderr(t *testing.T) {
	w, msg := scanErrorReport(models.NewEnforceError("uncovered categories: SCA"))

	assert.Equal(t, os.Stderr, w)
	assert.Contains(t, msg, "Enforcement failed")
	assert.Contains(t, msg, "SCA")
}

func TestScanErrorReportRoutesOperationalErrorToStdout(t *testing.T) {
	w, msg := scanErrorReport(errors.New("handler setup failed"))

	assert.Equal(t, os.Stdout, w)
	assert.Contains(t, msg, "scan repo failed")
	assert.Contains(t, msg, "handler setup failed")
}

func TestScanErrorReportUnwrapsEnforceError(t *testing.T) {
	wrapped := fmt.Errorf("execute: %w", models.NewEnforceError("uncovered categories: Coverage"))

	w, msg := scanErrorReport(wrapped)

	assert.Equal(t, os.Stderr, w)
	assert.Contains(t, msg, "Enforcement failed")
}

func TestFixRepo(t *testing.T) {
	err := fixRepo(&config.Config{})
	assert.NotNil(t, err, "fixRepo should return an error when the directory is not a repository")
}

// exitCode records what run would have exited with, -1 for not at all.
func exitCode(t *testing.T) *int {
	t.Helper()
	code := -1
	exit = func(c int) { code = c }
	t.Cleanup(func() { exit = os.Exit })
	return &code
}

func TestRunExitsWhenTheConfigurationIsBad(t *testing.T) {
	code := exitCode(t)
	did := false
	run(func(*cobra.Command) (*config.Config, error) { return nil, errors.New("no --dir") },
		func(*config.Config) error { did = true; return nil })(&cobra.Command{}, nil)

	assert.Equal(t, 1, *code)
	assert.False(t, did, "nothing runs without a configuration")
}

func TestRunExitsWhenTheWorkFails(t *testing.T) {
	code := exitCode(t)
	run(func(*cobra.Command) (*config.Config, error) { return &config.Config{}, nil },
		func(*config.Config) error { return models.NewEnforceError("uncovered categories: SCA") })(&cobra.Command{}, nil)

	assert.Equal(t, 1, *code)
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
