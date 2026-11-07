package handler

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/MustacheCase/zanadir/config"
	"github.com/MustacheCase/zanadir/grading"
	"github.com/MustacheCase/zanadir/internal/testrepo"
	"github.com/MustacheCase/zanadir/models"
	"github.com/MustacheCase/zanadir/output"
	"github.com/MustacheCase/zanadir/rules"
	"github.com/MustacheCase/zanadir/score"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAnalyseSCACoversDetectedLanguages(t *testing.T) {
	for _, tt := range []struct {
		name, steps, verdict string
		files                map[string]string
		missing              string
	}{
		{"Go only", "      - run: govulncheck ./...\n", "enforcing", map[string]string{"go.mod": "module example\n"}, ""},
		{"Python is unchecked", "      - run: govulncheck ./...\n", "partial", map[string]string{"go.mod": "", "requirements.txt": "requests\n"}, "Python"},
		{"complementary scanners", "      - run: govulncheck ./...\n      - run: pip-audit\n", "enforcing", map[string]string{"go.mod": "", "requirements.txt": ""}, ""},
		{"unrestricted enforcing scanner", "      - run: trivy fs --exit-code 1 .\n", "enforcing", map[string]string{"go.mod": "", "requirements.txt": ""}, ""},
		{"advisory unrestricted scanner", "      - run: govulncheck ./...\n      - run: trivy fs .\n", "partial", map[string]string{"go.mod": "", "requirements.txt": ""}, "Python"},
		{"advisory Python scanner", "      - run: govulncheck ./...\n      - run: pip-audit\n        continue-on-error: true\n", "partial", map[string]string{"go.mod": "", "requirements.txt": ""}, "Python"},
		{"dependency bot cannot close the gap", "      - run: govulncheck ./...\n", "partial", map[string]string{"go.mod": "", "requirements.txt": "", ".github/dependabot.yml": "version: 2\nupdates:\n  - package-ecosystem: pip\n    directory: /\n    schedule:\n      interval: daily\n"}, "Python"},
		{"tooling dependencies count", "      - run: govulncheck ./...\n", "partial", map[string]string{"go.mod": "", "package.json": `{"devDependencies":{"prettier":"3.0.0"}}`}, "JavaScript"},
		{"immediate subdirectory counts", "      - run: govulncheck ./...\n", "partial", map[string]string{"go.mod": "", "backend/requirements.txt": ""}, "Python"},
		{"dependency and hidden directories do not count", "      - run: govulncheck ./...\n", "enforcing", map[string]string{"go.mod": "", "node_modules/package.json": "{}", ".tools/requirements.txt": "", "services/api/requirements.txt": ""}, ""},
		{"no detected languages", "      - run: govulncheck ./...\n", "enforcing", nil, ""},
		{"existing advisory verdict", "      - run: govulncheck ./...\n        continue-on-error: true\n", "advisory", map[string]string{"go.mod": "", "requirements.txt": ""}, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			files := map[string]string{
				".git/HEAD":                "ref: refs/heads/main\n",
				".github/workflows/ci.yml": "on: pull_request\njobs:\n  scan:\n    runs-on: ubuntu-latest\n    steps:\n" + tt.steps,
			}
			for name, content := range tt.files {
				files[name] = content
			}
			dir := testrepo.Write(t, files)
			h, err := Setup()
			require.NoError(t, err)
			result, err := h.analyse(&config.Config{Dir: dir}, func(string, ...interface{}) {}, nil)
			require.NoError(t, err)
			index := slices.IndexFunc(result.covered, func(c output.Coverage) bool { return c.Category == "SCA" })
			require.NotEqual(t, -1, index)
			assert.Equal(t, tt.verdict, result.covered[index].Verdict)
			if tt.missing != "" {
				index := slices.IndexFunc(result.weaknesses, func(w output.Weakness) bool { return w.Category == "SCA" })
				require.NotEqual(t, -1, index)
				assert.Contains(t, result.weaknesses[index].Reason, tt.missing)
				assert.Contains(t, result.weaknesses[index].Reason, "manifests")
				assert.Equal(t, ".github/workflows/ci.yml", result.weaknesses[index].Location)
			} else if tt.verdict == "enforcing" {
				assert.False(t, slices.ContainsFunc(result.weaknesses, func(w output.Weakness) bool { return w.Category == "SCA" }))
			}
			for _, suggestion := range result.suggestions {
				assert.NotEqual(t, string(models.SCA), suggestion.ID, "SCA remains covered")
			}
		})
	}
}

func TestSCAEcosystemGapPreservesExistingReasonAndExclusions(t *testing.T) {
	dir := testrepo.Write(t, map[string]string{
		".git/HEAD": "ref: refs/heads/main\n",
		"go.mod":    "", "requirements.txt": "",
		"uv.lock":                  "[[package]]\nname = \"forked\"\nsource = { git = \"https://example.com/forked\" }\n",
		".github/workflows/ci.yml": "on: pull_request\njobs:\n  scan:\n    steps:\n      - run: govulncheck ./...\n",
	})
	h, err := Setup()
	require.NoError(t, err)
	result, err := h.analyse(&config.Config{Dir: dir}, func(string, ...interface{}) {}, nil)
	require.NoError(t, err)
	require.Len(t, result.weaknesses, 1)
	assert.Contains(t, result.weaknesses[0].Reason, "uv.lock installs forked")
	result, err = h.analyse(&config.Config{Dir: dir, ExcludedCategories: []string{"SCA"}}, func(string, ...interface{}) {}, nil)
	require.NoError(t, err)
	assert.Empty(t, result.weaknesses)
	assert.False(t, slices.ContainsFunc(result.covered, func(c output.Coverage) bool { return c.Category == "SCA" }))
}

func TestCustomSCAEcosystemsAffectGradingAndEnforcement(t *testing.T) {
	dir := testrepo.Write(t, map[string]string{
		".git/HEAD": "ref: refs/heads/main\n",
		"go.mod":    "", "requirements.txt": "",
		".github/workflows/ci.yml": "on: pull_request\njobs:\n  scan:\n    steps:\n      - run: acme-scan\n",
	})
	for _, ecosystems := range [][]string{{"Go"}, {"Go", "Python"}, nil} {
		h, err := Setup(rules.FileRule{ID: "acme", Name: "Acme", ApplyOn: []string{rules.FieldJobRun}, Categories: []string{"SCA"}, Regex: "acme-scan", Ecosystems: ecosystems})
		require.NoError(t, err)
		cfg := &config.Config{Dir: dir, Output: "json", Out: &bytes.Buffer{}, FailOn: []string{"SCA"}, FailOnWeakened: true}
		err = h.Execute(cfg)
		if len(ecosystems) == 1 {
			assert.ErrorContains(t, err, "SCA (partial)")
		} else {
			assert.NoError(t, err)
		}
		cfg.FailOnWeakened = false
		assert.NoError(t, h.Execute(cfg), "presence-based enforcement remains unchanged")
	}
}

func TestSCAEcosystemGapDoesNotChangeCoverageScore(t *testing.T) {
	dir := testrepo.Write(t, map[string]string{
		".git/HEAD": "ref: refs/heads/main\n", "go.mod": "",
		".github/workflows/ci.yml": "on: pull_request\njobs:\n  scan:\n    steps:\n      - run: govulncheck ./...\n",
	})
	h, err := Setup()
	require.NoError(t, err)
	before, err := h.analyse(&config.Config{Dir: dir}, func(string, ...interface{}) {}, nil)
	require.NoError(t, err)
	// A new manifest changes SCA effectiveness, not whether the category exists.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "requirements.txt"), nil, 0o600))
	after, err := h.analyse(&config.Config{Dir: dir}, func(string, ...interface{}) {}, nil)
	require.NoError(t, err)
	var beforeIDs, afterIDs []string
	for _, suggestion := range before.suggestions {
		beforeIDs = append(beforeIDs, suggestion.ID)
	}
	for _, suggestion := range after.suggestions {
		afterIDs = append(afterIDs, suggestion.ID)
	}
	assert.Equal(t, beforeIDs, afterIDs)
	assert.Equal(t, score.Of(before.skipped, len(before.suggestions)), score.Of(after.skipped, len(after.suggestions)))
	require.Len(t, after.weaknesses, 1)
	assert.Equal(t, string(grading.Partial), after.weaknesses[0].Verdict)
}
