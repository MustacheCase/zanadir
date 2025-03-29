package app

import (
	"testing"

	"github.com/MustacheCase/zanadir/config"
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
	assert.Equal(t, "Path to the GitHub repository directory (required)", dirFlag.Usage)

	excludedCategoriesFlag := cmd.Flags().Lookup("excluded-categories")
	assert.NotNil(t, excludedCategoriesFlag, "The 'excluded-categories' flag should be defined")
	assert.Equal(t, "List of excluded categories (optional)", excludedCategoriesFlag.Usage)

	enforceFlag := cmd.Flags().Lookup("enforce")
	assert.NotNil(t, enforceFlag, "The 'enforce' flag should be defined")
	assert.Equal(t, "Fails the CI process when at least one rule is met (optional)", enforceFlag.Usage)
}

func TestScanRepo(t *testing.T) {
	mockConfig := &config.Config{}
	err := scanRepo(mockConfig)
	assert.NotNil(t, err, "scanRepo should return an error when handler setup fails")
}