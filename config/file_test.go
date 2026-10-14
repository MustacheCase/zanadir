package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), ".zanadir.yml")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	return path
}

func withConfig(cmd *cobra.Command, path string) *cobra.Command {
	cmd.Flags().StringP("config", "c", "", "config")
	_ = cmd.Flags().Set("config", path)
	return cmd
}

func TestConfigFileSetsScanFlags(t *testing.T) {
	path := writeConfig(t, `
excluded-categories: [performance testing]
enforce: true
fail-on: [sca, secrets detection]
fail-on-weakened: true
output: markdown
badge: badge.json
baseline: /abs/baseline.yaml
`)
	cmd := withConfig(newScanCmd(t.TempDir(), nil), path)
	cmd.Flags().String("output-file", "", "output file")

	cfg, err := CreateConfig(cmd)
	require.NoError(t, err)
	assert.Equal(t, []string{"Performance Testing"}, cfg.ExcludedCategories)
	assert.True(t, cfg.Enforce)
	assert.Equal(t, []string{"SCA", "Secrets Detection"}, cfg.FailOn)
	assert.True(t, cfg.FailOnWeakened)
	assert.Equal(t, OutputMarkdown, cfg.Output)
	assert.Equal(t, filepath.Join(filepath.Dir(path), "badge.json"), cfg.Badge)
	assert.Equal(t, "/abs/baseline.yaml", cfg.Baseline)
	assert.Empty(t, cfg.OutputFile)
}

func TestCommandLineWinsOverConfigFile(t *testing.T) {
	path := writeConfig(t, "excluded-categories: [linter]\nenforce: true\noutput: json\n")
	cmd := withConfig(newScanCmd(t.TempDir(), nil), path)
	require.NoError(t, cmd.Flags().Set("excluded-categories", "sca"))
	require.NoError(t, cmd.Flags().Set("enforce", "false"))

	cfg, err := CreateConfig(cmd)
	require.NoError(t, err)
	assert.Equal(t, []string{"SCA"}, cfg.ExcludedCategories)
	assert.False(t, cfg.Enforce)
	assert.Equal(t, OutputJSON, cfg.Output)
}

func TestConfigFileRejectsUnknownKeys(t *testing.T) {
	path := writeConfig(t, "exclude-categories: [linter]\n")
	_, err := CreateConfig(withConfig(newScanCmd(t.TempDir(), nil), path))
	assert.ErrorContains(t, err, "exclude-categories")
}

func TestConfigFileCategoriesAreValidated(t *testing.T) {
	path := writeConfig(t, "fail-on: [Lintr]\n")
	_, err := CreateConfig(withConfig(newScanCmd(t.TempDir(), nil), path))
	assert.ErrorContains(t, err, "unknown category")
}

func TestConfigFileRejectsAWrongType(t *testing.T) {
	path := writeConfig(t, "enforce: sometimes\n")
	_, err := CreateConfig(withConfig(newScanCmd(t.TempDir(), nil), path))
	assert.Error(t, err)
}

func TestMissingConfigFileIsAnError(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope.yml")
	_, err := CreateConfig(withConfig(newScanCmd(t.TempDir(), nil), missing))
	assert.ErrorContains(t, err, "nope.yml")
}

func TestEmptyConfigFileChangesNothing(t *testing.T) {
	path := writeConfig(t, "")
	cfg, err := CreateConfig(withConfig(newScanCmd(t.TempDir(), nil), path))
	require.NoError(t, err)
	assert.Empty(t, cfg.ExcludedCategories)
	assert.False(t, cfg.Enforce)
	assert.Equal(t, OutputTable, cfg.Output)
}

func TestFixIgnoresScanOnlyKeys(t *testing.T) {
	path := writeConfig(t, "excluded-categories: [linter]\nenforce: true\nbadge: b.json\n")
	cfg, err := CreateFixConfig(withConfig(newFixCmd(t.TempDir()), path))
	require.NoError(t, err)
	assert.Equal(t, []string{"Linter"}, cfg.ExcludedCategories)
	assert.False(t, cfg.Enforce)
	assert.Empty(t, cfg.Badge)
}

func TestConfigFileValueTheFlagRejects(t *testing.T) {
	path := writeConfig(t, "enforce: true\n")
	cmd := &cobra.Command{}
	cmd.Flags().String("dir", t.TempDir(), "dir")
	cmd.Flags().Int("enforce", 0, "not a bool")
	_, err := CreateConfig(withConfig(cmd, path))
	assert.ErrorContains(t, err, "enforce")
}

func TestFixRejectsABadConfigFile(t *testing.T) {
	path := writeConfig(t, "exclude-categories: [linter]\n")
	cfg, err := CreateFixConfig(withConfig(newFixCmd(t.TempDir()), path))
	assert.Nil(t, cfg)
	assert.ErrorContains(t, err, "exclude-categories")
}

func TestConfigFileCarriesCustomRules(t *testing.T) {
	path := writeConfig(t, `
rules:
  - id: acme-scan
    applyOn: [Job.Run]
    categories: [SAST]
    regex: '\bacme-scan\b'
    weakenedWhen:
      - runMatches: '--report-only'
        verdict: advisory
`)
	scan, err := CreateConfig(withConfig(newScanCmd(t.TempDir(), nil), path))
	require.NoError(t, err)
	require.Len(t, scan.Rules, 1)
	assert.Equal(t, "acme-scan", scan.Rules[0].ID)
	assert.Equal(t, "advisory", scan.Rules[0].Weakened[0].Verdict)

	fix, err := CreateFixConfig(withConfig(newFixCmd(t.TempDir()), path))
	require.NoError(t, err)
	assert.Equal(t, scan.Rules, fix.Rules)
}

func TestConfigFileRejectsAMisspeltRuleKey(t *testing.T) {
	path := writeConfig(t, "rules:\n  - id: acme\n    aplyOn: [Job.Run]\n")
	_, err := CreateConfig(withConfig(newScanCmd(t.TempDir(), nil), path))
	assert.ErrorContains(t, err, "aplyOn")
}
