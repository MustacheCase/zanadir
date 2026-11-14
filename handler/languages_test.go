package handler

import (
	"slices"
	"testing"

	"github.com/MustacheCase/zanadir/config"
	"github.com/MustacheCase/zanadir/internal/testrepo"
	"github.com/MustacheCase/zanadir/models"
	"github.com/MustacheCase/zanadir/output"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAnalyseHoldsCodeQLToTheDetectedLanguages(t *testing.T) {
	workflow := func(languages string) map[string]string {
		return map[string]string{".github/workflows/codeql.yml": "on: pull_request\njobs:\n  analyze:\n    runs-on: ubuntu-latest\n    steps:\n" +
			"      - uses: github/codeql-action/init@v3\n        with:\n          languages: " + languages + "\n      - uses: github/codeql-action/analyze@v3\n"}
	}
	defaultSetup := func(languages string) map[string]string {
		return map[string]string{models.CodeScanningFile: languages}
	}
	rustAndKotlin := map[string]string{"Cargo.toml": "", "app/build.gradle.kts": ""}
	for _, tt := range []struct {
		name     string
		ci, code map[string]string
		verdict  string
		location string
	}{
		{"workflow misses the code", workflow("actions, python"), rustAndKotlin, "partial", ".github/workflows/codeql.yml"},
		{"workflow covers Kotlin", workflow("java-kotlin"), rustAndKotlin, "enforcing", ""},
		{"a matrix of languages", workflow("${{ matrix.language }}"), rustAndKotlin, "enforcing", ""},
		{"a pyproject without dependencies is still Python code", workflow("python"), map[string]string{"pyproject.toml": "[tool.ruff]\n"}, "enforcing", ""},
		{"default setup misses the code", defaultSetup("python\nactions\n"), rustAndKotlin, "partial", ""},
		{"default setup covers Rust", defaultSetup("actions\nrust\n"), rustAndKotlin, "enforcing", ""},
		{"default setup on actions alone", defaultSetup("actions\n"), rustAndKotlin, "", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			files := map[string]string{".git/HEAD": "ref: refs/heads/main\n"}
			for name, content := range tt.ci {
				files[name] = content
			}
			for name, content := range tt.code {
				files[name] = content
			}
			h, err := Setup()
			require.NoError(t, err)
			result, err := h.analyse(&config.Config{Dir: testrepo.Write(t, files)}, func(string, ...interface{}) {}, nil)
			require.NoError(t, err)
			index := slices.IndexFunc(result.covered, func(c output.Coverage) bool { return c.Category == string(models.SAST) })
			if tt.verdict == "" {
				assert.Equal(t, -1, index, "actions alone checks workflows, not code")
				return
			}
			require.NotEqual(t, -1, index)
			assert.Equal(t, tt.verdict, result.covered[index].Verdict)
			weakness := slices.IndexFunc(result.weaknesses, func(w output.Weakness) bool { return w.Category == string(models.SAST) })
			if tt.verdict == "enforcing" {
				assert.Equal(t, -1, weakness)
				return
			}
			require.NotEqual(t, -1, weakness)
			assert.Equal(t, tt.location, result.weaknesses[weakness].Location)
			assert.Equal(t, "CodeQL analyses only actions, python; the Java, Rust code detected from repository manifests is not scanned", result.weaknesses[weakness].Reason)
		})
	}
}
