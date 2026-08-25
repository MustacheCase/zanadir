package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MustacheCase/zanadir/baseline"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestCreateConfig(t *testing.T) {
	tests := []struct {
		name               string
		dir                string
		excludedCategories []string
		expectError        bool
	}{
		{
			name:               "Valid directory",
			dir:                "/tmp/testdir",
			excludedCategories: []string{},
			expectError:        false,
		},
		{
			name:               "Empty directory flag",
			dir:                "",
			excludedCategories: []string{},
			expectError:        true,
		},
		{
			name:               "Symlink directory",
			dir:                "/tmp/symlinkdir",
			excludedCategories: []string{},
			expectError:        true,
		},
		{
			name:               "Excluded categories",
			dir:                "/tmp/testdir",
			excludedCategories: []string{"SCA", "Linter"},
			expectError:        false,
		},
		{
			name:               "Unknown excluded category",
			dir:                "/tmp/testdir",
			excludedCategories: []string{"cat1"},
			expectError:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			cmd.Flags().String("output", "json", "output format")
			cmd.Flags().String("dir", tt.dir, "directory")
			cmd.Flags().StringSlice("excluded-categories", tt.excludedCategories, "excluded categories")

			if tt.dir == "/tmp/symlinkdir" {
				_ = os.Symlink("/tmp/testdir", "/tmp/symlinkdir")
				defer os.Remove("/tmp/symlinkdir") //nolint:errcheck
			} else if tt.dir != "" {
				_ = os.MkdirAll(tt.dir, os.ModePerm)
				defer os.RemoveAll(tt.dir) //nolint:errcheck
			}

			config, err := CreateConfig(cmd)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, filepath.Clean(tt.dir), config.Dir)
				assert.Equal(t, tt.excludedCategories, config.ExcludedCategories)
			}
		})
	}

	// Additional subtest to improve coverage for config/config.go#L48-L49.
	t.Run("unsupported output", func(t *testing.T) {
		cmd := &cobra.Command{}
		// Set an unsupported output value to trigger error.
		cmd.Flags().String("output", "xml", "output format")
		cmd.Flags().String("dir", "/tmp/testdir", "directory")
		cmd.Flags().StringSlice("excluded-categories", []string{}, "excluded categories")
		_ = os.MkdirAll("/tmp/testdir", os.ModePerm)
		defer os.RemoveAll("/tmp/testdir") //nolint:errcheck
		config, err := CreateConfig(cmd)
		assert.Error(t, err)
		assert.Nil(t, config)
	})
}

func newScanCmd(dir string, excluded []string) *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().String("dir", dir, "directory")
	cmd.Flags().StringSlice("excluded-categories", excluded, "excluded categories")
	cmd.Flags().Bool("enforce", false, "enforce")
	cmd.Flags().Bool("debug", false, "debug")
	cmd.Flags().String("output", OutputTable, "output")
	cmd.Flags().StringSlice("fail-on", nil, "fail on")
	cmd.Flags().String("baseline", "", "baseline")
	cmd.Flags().Bool("write-baseline", false, "write baseline")
	return cmd
}

func newFailOnCmd(dir string, failOn []string) *cobra.Command {
	cmd := newScanCmd(dir, nil)
	_ = cmd.Flags().Set("fail-on", strings.Join(failOn, ","))
	return cmd
}

// An unvalidated --fail-on silently enforces nothing, so a typo turns CI green
// exactly when it should go red.
func TestCreateConfigRejectsUnknownFailOnCategory(t *testing.T) {
	cfg, err := CreateConfig(newFailOnCmd(t.TempDir(), []string{"Lintr"}))
	assert.Error(t, err)
	assert.Nil(t, cfg)
	assert.Contains(t, err.Error(), "unknown category")
}

func TestCreateConfigNormalizesFailOn(t *testing.T) {
	cfg, err := CreateConfig(newFailOnCmd(t.TempDir(), []string{"sca", "secrets detection"}))
	assert.NoError(t, err)
	assert.Equal(t, []string{"SCA", "Secrets Detection"}, cfg.FailOn)
}

func TestNormalizeCategories(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected []string
		wantErr  bool
	}{
		{name: "empty", input: nil, expected: []string{}},
		{name: "canonical name", input: []string{"Secrets Detection"}, expected: []string{"Secrets Detection"}},
		{name: "case insensitive", input: []string{"secrets detection"}, expected: []string{"Secrets Detection"}},
		{name: "surrounding whitespace", input: []string{" SCA "}, expected: []string{"SCA"}},
		{name: "multiple", input: []string{"SCA", "Linter"}, expected: []string{"SCA", "Linter"}},
		// "Secrets" is the old id, not a category title.
		{name: "stale short name is rejected", input: []string{"Secrets"}, wantErr: true},
		{name: "typo is rejected", input: []string{"Lintr"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeCategories(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestCreateConfigRejectsUnknownCategory(t *testing.T) {
	dir := t.TempDir()
	cfg, err := CreateConfig(newScanCmd(dir, []string{"NotACategory"}))
	assert.Error(t, err)
	assert.Nil(t, cfg)
	assert.Contains(t, err.Error(), "unknown category")
}

func TestCreateConfigNormalizesExcludedCategories(t *testing.T) {
	dir := t.TempDir()
	cfg, err := CreateConfig(newScanCmd(dir, []string{"sca"}))
	assert.NoError(t, err)
	assert.Equal(t, []string{"SCA"}, cfg.ExcludedCategories)
}

func TestCreateConfigDefaultsBaselinePathForWrite(t *testing.T) {
	cmd := newScanCmd(t.TempDir(), nil)
	assert.NoError(t, cmd.Flags().Set("write-baseline", "true"))

	cfg, err := CreateConfig(cmd)
	assert.NoError(t, err)
	assert.True(t, cfg.WriteBaseline)
	assert.Equal(t, baseline.DefaultPath, cfg.Baseline)
}

func newFixCmd(dir string) *cobra.Command {
	cmd := &cobra.Command{Use: "fix"}
	cmd.Flags().StringP("dir", "d", dir, "dir")
	cmd.Flags().StringSliceP("excluded-categories", "e", nil, "excluded")
	cmd.Flags().Bool("debug", false, "debug")
	return cmd
}

func TestCreateFixConfig(t *testing.T) {
	dir := t.TempDir()
	cfg, err := CreateFixConfig(newFixCmd(dir))

	assert.NoError(t, err)
	assert.Equal(t, dir, cfg.Dir)
	assert.Empty(t, cfg.ExcludedCategories)
	assert.False(t, cfg.Debug)
}

func TestCreateFixConfigNormalizesExclusions(t *testing.T) {
	cmd := newFixCmd(t.TempDir())
	assert.NoError(t, cmd.Flags().Set("excluded-categories", "sca"))

	cfg, err := CreateFixConfig(cmd)

	assert.NoError(t, err)
	assert.Equal(t, []string{"SCA"}, cfg.ExcludedCategories)
}

func TestCreateFixConfigRejectsUnknownExclusion(t *testing.T) {
	cmd := newFixCmd(t.TempDir())
	assert.NoError(t, cmd.Flags().Set("excluded-categories", "Lintr"))

	cfg, err := CreateFixConfig(cmd)

	assert.Error(t, err)
	assert.Nil(t, cfg)
}

func TestCreateFixConfigRequiresDir(t *testing.T) {
	cfg, err := CreateFixConfig(newFixCmd(""))

	assert.Error(t, err)
	assert.Nil(t, cfg)
}
