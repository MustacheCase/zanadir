package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MustacheCase/zanadir/baseline"
	"github.com/MustacheCase/zanadir/remote"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	cmd.Flags().String("badge", "", "badge")
	cmd.Flags().Bool("fail-on-weakened", false, "fail on weakened")
	cmd.Flags().Bool("required-checks", false, "required checks")
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
	dir := t.TempDir()
	cmd := newScanCmd(dir, nil)
	assert.NoError(t, cmd.Flags().Set("write-baseline", "true"))

	cfg, err := CreateConfig(cmd)
	assert.NoError(t, err)
	assert.True(t, cfg.WriteBaseline)
	assert.Equal(t, filepath.Join(dir, baseline.DefaultPath), cfg.Baseline, "in the scanned directory, not the current one")
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

func TestCreateConfigReadsTheBadgePath(t *testing.T) {
	cmd := newScanCmd(t.TempDir(), nil)
	assert.NoError(t, cmd.Flags().Set("badge", "badge.json"))

	cfg, err := CreateConfig(cmd)
	assert.NoError(t, err)
	assert.Equal(t, "badge.json", cfg.Badge)
}

func TestCreateConfigDefaultsToNoBadge(t *testing.T) {
	cfg, err := CreateConfig(newScanCmd(t.TempDir(), nil))
	assert.NoError(t, err)
	assert.Empty(t, cfg.Badge)
}

func TestCreateConfigAcceptsMarkdown(t *testing.T) {
	cmd := newScanCmd(t.TempDir(), nil)
	assert.NoError(t, cmd.Flags().Set("output", OutputMarkdown))

	cfg, err := CreateConfig(cmd)
	assert.NoError(t, err)
	assert.Equal(t, OutputMarkdown, cfg.Output)
}

func TestCreateConfigListsEveryFormatWhenRejecting(t *testing.T) {
	cmd := newScanCmd(t.TempDir(), nil)
	assert.NoError(t, cmd.Flags().Set("output", "xml"))

	_, err := CreateConfig(cmd)
	assert.Error(t, err)
	for _, format := range OutputFormats {
		assert.Contains(t, err.Error(), format)
	}
}

func TestCreateConfigReadsFailOnWeakened(t *testing.T) {
	cmd := newScanCmd(t.TempDir(), nil)
	assert.NoError(t, cmd.Flags().Set("fail-on-weakened", "true"))

	cfg, err := CreateConfig(cmd)
	assert.NoError(t, err)
	assert.True(t, cfg.FailOnWeakened)
	assert.False(t, cfg.RequiredChecks, "reading merge settings calls GitHub, so it is opt-in")

	assert.NoError(t, cmd.Flags().Set("required-checks", "true"))
	cfg, err = CreateConfig(cmd)
	assert.NoError(t, err)
	assert.True(t, cfg.RequiredChecks)
}

// Off by default: a weakened control must not start failing builds on upgrade.
func TestCreateConfigDefaultsToNotFailingOnWeakened(t *testing.T) {
	cfg, err := CreateConfig(newScanCmd(t.TempDir(), nil))
	assert.NoError(t, err)
	assert.False(t, cfg.FailOnWeakened)
}

func TestCreateConfigRejectsAMissingDirectory(t *testing.T) {
	_, err := CreateConfig(newScanCmd(filepath.Join(t.TempDir(), "missing"), nil))
	assert.Error(t, err)
}

func newRepoCmd(dir, repo string) *cobra.Command {
	cmd := newScanCmd(dir, nil)
	cmd.Flags().String("repo", repo, "repo")
	return cmd
}

func TestCreateConfigReadsARemoteRepository(t *testing.T) {
	cfg, err := CreateConfig(newRepoCmd("", "github.com/cli/cli@v2"))
	require.NoError(t, err)
	assert.Empty(t, cfg.Dir)
	assert.Equal(t, &remote.Repository{Owner: "cli", Name: "cli", Ref: "v2"}, cfg.Repo)
}

func TestCreateConfigTakesOneRepository(t *testing.T) {
	_, err := CreateConfig(newRepoCmd(t.TempDir(), "cli/cli"))
	assert.ErrorContains(t, err, "give one")

	_, err = CreateConfig(newRepoCmd("", "gitlab.com/a/b"))
	assert.ErrorContains(t, err, "only github.com")

	_, err = CreateConfig(newRepoCmd("", ""))
	assert.ErrorContains(t, err, "--dir (-d) or --repo is required")
}

func TestCreateFixConfigWillNotWriteIntoAFetchedRepository(t *testing.T) {
	cmd := newFixCmd("")
	cmd.Flags().String("repo", "cli/cli", "repo")
	cmd.Flags().Bool("write", true, "write")
	_, err := CreateFixConfig(cmd)
	assert.ErrorContains(t, err, "--write adds a workflow to a checkout")
}

func TestCreateConfigReadsTheOutputFormatInAnyCase(t *testing.T) {
	cmd := newScanCmd(t.TempDir(), nil)
	assert.NoError(t, cmd.Flags().Set("output", "JSON"))
	cfg, err := CreateConfig(cmd)
	assert.NoError(t, err)
	assert.Equal(t, "json", cfg.Output)
}

// A symlink the operator names is followed; repofs keeps reads inside it.
func TestCreateConfigFollowsASymlinkedDir(t *testing.T) {
	dir := t.TempDir()
	link := filepath.Join(t.TempDir(), "link")
	assert.NoError(t, os.Symlink(dir, link))
	cfg, err := CreateConfig(newScanCmd(link, nil))
	assert.NoError(t, err)
	assert.Equal(t, link, cfg.Dir)

	file := filepath.Join(dir, "file")
	assert.NoError(t, os.WriteFile(file, nil, 0o600))
	_, err = CreateConfig(newScanCmd(file, nil))
	assert.ErrorContains(t, err, "is not a directory")
}

func newOrgCmd(org string) *cobra.Command {
	cmd := newScanCmd("", nil)
	cmd.Flags().String("repo", "", "repo")
	cmd.Flags().String("org", org, "org")
	cmd.Flags().Int("parallel", 4, "parallel")
	cmd.Flags().Bool("include-archived", false, "archived")
	cmd.Flags().Bool("include-forks", false, "forks")
	return cmd
}

func TestCreateConfigForAnOrganisation(t *testing.T) {
	cmd := newOrgCmd("github.com/acme")
	assert.NoError(t, cmd.Flags().Set("parallel", "8"))
	assert.NoError(t, cmd.Flags().Set("include-forks", "true"))
	assert.NoError(t, cmd.Flags().Set("output", "json"))
	cfg, err := CreateConfig(cmd)
	assert.NoError(t, err)
	assert.Equal(t, "acme", cfg.Org)
	assert.Equal(t, 8, cfg.Parallel)
	assert.True(t, cfg.IncludeForks)
	assert.False(t, cfg.IncludeArchived)
	assert.Empty(t, cfg.Dir)
}

func TestCreateConfigRefusesWhatOrgDoesNotSupportYet(t *testing.T) {
	for flag, value := range map[string]string{
		"enforce": "true", "fail-on": "SAST", "fail-on-weakened": "true", "required-checks": "true",
		"baseline": "b.yaml", "write-baseline": "true", "badge": "b.json", "output": "sarif", "parallel": "0",
	} {
		cmd := newOrgCmd("acme")
		assert.NoError(t, cmd.Flags().Set(flag, value))
		_, err := CreateConfig(cmd)
		assert.Error(t, err, flag)
	}
	for _, other := range []string{"dir", "repo"} {
		cmd := newOrgCmd("acme")
		assert.NoError(t, cmd.Flags().Set(other, "x"))
		_, err := CreateConfig(cmd)
		assert.ErrorContains(t, err, "without --dir or --repo", other)
	}
	_, err := CreateConfig(newOrgCmd("acme/repo"))
	assert.ErrorContains(t, err, "not an organisation")
}
