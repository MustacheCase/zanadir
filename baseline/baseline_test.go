package baseline_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/MustacheCase/zanadir/baseline"
	"github.com/stretchr/testify/assert"
)

// A missing baseline must not error, or the first enforcing run would break.
func TestLoadMissingFileIsNotAnError(t *testing.T) {
	b, err := baseline.Load(filepath.Join(t.TempDir(), "absent.yaml"))
	assert.NoError(t, err)
	assert.NotNil(t, b)
	assert.Empty(t, b.Categories)
	assert.False(t, b.Contains("SCA"))
}

func TestWriteThenLoadRoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), baseline.DefaultPath)

	assert.NoError(t, baseline.Write(path, []string{"Coverage", "SCA"}, nil))

	b, err := baseline.Load(path)
	assert.NoError(t, err)
	assert.Equal(t, baseline.Version, b.Version)
	assert.Equal(t, []string{"Coverage", "SCA"}, b.Categories)
	assert.True(t, b.Contains("SCA"))
	assert.False(t, b.Contains("Linter"))
}

func TestWriteSortsForStableDiffs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "b.yaml")
	assert.NoError(t, baseline.Write(path, []string{"Linter", "Coverage", "SCA"}, nil))

	b, err := baseline.Load(path)
	assert.NoError(t, err)
	assert.Equal(t, []string{"Coverage", "Linter", "SCA"}, b.Categories)
}

func TestWriteIncludesExplanatoryHeader(t *testing.T) {
	path := filepath.Join(t.TempDir(), "b.yaml")
	assert.NoError(t, baseline.Write(path, []string{"SCA"}, nil))

	data, err := os.ReadFile(path)
	assert.NoError(t, err)
	assert.Contains(t, string(data), "# zanadir baseline")
	assert.Contains(t, string(data), "--write-baseline")
}

func TestContainsIsCaseInsensitive(t *testing.T) {
	b := &baseline.Baseline{Version: baseline.Version, Categories: []string{"  sca  ", "Unit Tests"}}
	assert.True(t, b.Contains("SCA"))
	assert.True(t, b.Contains("unit tests"))
	assert.False(t, b.Contains("Coverage"))
}

func TestLoadRejectsUnknownVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "b.yaml")
	assert.NoError(t, os.WriteFile(path, []byte("version: 99\ncategories: []\n"), 0o600))

	_, err := baseline.Load(path)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported version")
}

func TestLoadRejectsMalformedYAML(t *testing.T) {
	path := filepath.Join(t.TempDir(), "b.yaml")
	assert.NoError(t, os.WriteFile(path, []byte("categories: [oops\n"), 0o600))

	_, err := baseline.Load(path)
	assert.Error(t, err)
}

func TestNilBaselineContainsNothing(t *testing.T) {
	var b *baseline.Baseline
	assert.False(t, b.Contains("SCA"))
}

func TestLoadReportsUnreadablePath(t *testing.T) {
	_, err := baseline.Load(t.TempDir())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read baseline")
}

func TestWriteReportsUnwritablePath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "no-such-dir", "b.yaml")

	err := baseline.Write(path, []string{"SCA"}, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to write baseline")
}

func TestWriteAndLoadWeakenedVerdicts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "base.yaml")

	assert.NoError(t, baseline.Write(path, []string{"SCA"}, map[string]string{
		"Secrets Detection": "partial",
		"Linter":            "advisory",
	}))

	loaded, err := baseline.Load(path)
	assert.NoError(t, err)
	assert.Equal(t, []string{"SCA"}, loaded.Categories)

	verdict, ok := loaded.AcceptedVerdict("Secrets Detection")
	assert.True(t, ok)
	assert.Equal(t, "partial", verdict)

	verdict, ok = loaded.AcceptedVerdict("Linter")
	assert.True(t, ok)
	assert.Equal(t, "advisory", verdict)

	_, ok = loaded.AcceptedVerdict("SAST")
	assert.False(t, ok)
}

// Category names come from a file a person edits.
func TestAcceptedVerdictIsCaseInsensitive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "base.yaml")
	assert.NoError(t, baseline.Write(path, nil, map[string]string{"  secrets detection ": "advisory"}))

	loaded, err := baseline.Load(path)
	assert.NoError(t, err)

	verdict, ok := loaded.AcceptedVerdict("Secrets Detection")
	assert.True(t, ok)
	assert.Equal(t, "advisory", verdict)
}

// A missing baseline file accepts nothing rather than everything.
func TestAcceptedVerdictWithNoBaseline(t *testing.T) {
	var absent *baseline.Baseline
	_, ok := absent.AcceptedVerdict("SCA")
	assert.False(t, ok)

	loaded, err := baseline.Load(filepath.Join(t.TempDir(), "absent.yaml"))
	assert.NoError(t, err)
	_, ok = loaded.AcceptedVerdict("SCA")
	assert.False(t, ok)
}

// The weakened section is omitted when there is nothing in it, so a baseline
// written before verdicts existed keeps loading and keeps looking the same.
func TestWriteOmitsAnEmptyWeakenedSection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "base.yaml")
	assert.NoError(t, baseline.Write(path, []string{"SCA"}, nil))

	data, err := os.ReadFile(path)
	assert.NoError(t, err)
	// The key, not the word: the header explains what the section is for.
	assert.NotContains(t, string(data), "weakened:")

	loaded, err := baseline.Load(path)
	assert.NoError(t, err)
	assert.Equal(t, []string{"SCA"}, loaded.Categories)
}
