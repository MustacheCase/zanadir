package fixer

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MustacheCase/zanadir/suggester"
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
)

func TestEmbeddedTemplatesAreValid(t *testing.T) {
	_, err := NewFixService()
	assert.NoError(t, err, "shipped templates must resolve against suggestions.yaml")
}

func TestValidateRejectsUnknownTool(t *testing.T) {
	catalogue := []suggester.CategorySuggestion{
		{ID: "Secrets Detection", Suggestions: []*suggester.Suggestion{{Name: "Gitleaks"}}},
	}
	err := validate([]Template{{Tool: "Gitlekas", Platform: "github", Step: "- run: x"}}, catalogue)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Gitlekas")
}

func TestValidateRejectsUnknownPlatform(t *testing.T) {
	catalogue := []suggester.CategorySuggestion{
		{ID: "Secrets Detection", Suggestions: []*suggester.Suggestion{{Name: "Gitleaks"}}},
	}
	err := validate([]Template{{Tool: "Gitleaks", Platform: "travis", Step: "- run: x"}}, catalogue)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "travis")
}

func TestValidateRejectsEmptyStep(t *testing.T) {
	catalogue := []suggester.CategorySuggestion{
		{ID: "Secrets Detection", Suggestions: []*suggester.Suggestion{{Name: "Gitleaks"}}},
	}
	err := validate([]Template{{Tool: "Gitleaks", Platform: "github", Step: ""}}, catalogue)

	assert.Error(t, err)
}

func testFixer(t *testing.T) Fixer {
	t.Helper()
	f, err := NewFixService()
	assert.NoError(t, err)
	return f
}

func TestSnippetsOnePerCategory(t *testing.T) {
	suggestions := []*suggester.CategorySuggestion{
		{
			ID: "Secrets Detection", Name: "Data Leakage & Secrets Detection",
			Suggestions: []*suggester.Suggestion{
				{Name: "Gitleaks", Repository: "https://github.com/gitleaks/gitleaks"},
				{Name: "TruffleHog", Repository: "https://github.com/trufflesecurity/trufflehog"},
			},
		},
	}

	snippets := testFixer(t).Snippets(suggestions, PlatformGitHub)

	assert.Len(t, snippets, 1, "one snippet per category, not per tool")
	assert.Equal(t, "Gitleaks", snippets[0].Tool)
	assert.Contains(t, snippets[0].Step, "gitleaks-action")
	assert.NotEmpty(t, snippets[0].Repository)
}

func TestSnippetsFallsThroughToATemplatedTool(t *testing.T) {
	suggestions := []*suggester.CategorySuggestion{
		{
			ID: "SCA", Name: "SCA Open Source Tools",
			Suggestions: []*suggester.Suggestion{
				{Name: "Snyk"}, // no template
				{Name: "Trivy"},
			},
		},
	}

	snippets := testFixer(t).Snippets(suggestions, PlatformGitHub)

	assert.Len(t, snippets, 1)
	assert.Equal(t, "Trivy", snippets[0].Tool)
}

func TestSnippetsSkipsUntemplatedCategory(t *testing.T) {
	suggestions := []*suggester.CategorySuggestion{
		{ID: "Unit Tests", Name: "Unit Tests",
			Suggestions: []*suggester.Suggestion{{Name: "JUnit"}, {Name: "pytest"}}},
	}

	assert.Empty(t, testFixer(t).Snippets(suggestions, PlatformGitHub),
		"a category with no template is skipped rather than guessed at")
}

func TestSnippetsArePlatformSpecific(t *testing.T) {
	suggestions := []*suggester.CategorySuggestion{
		{ID: "Secrets Detection", Name: "Secrets",
			Suggestions: []*suggester.Suggestion{{Name: "Gitleaks"}}},
	}

	gh := testFixer(t).Snippets(suggestions, PlatformGitHub)
	gl := testFixer(t).Snippets(suggestions, PlatformGitLab)

	assert.Contains(t, gh[0].Step, "uses: gitleaks/gitleaks-action")
	assert.Contains(t, gl[0].Step, "secret_detection:")
	assert.NotContains(t, gl[0].Step, "uses:")
}

func TestSnippetsEmptyForCircleCI(t *testing.T) {
	suggestions := []*suggester.CategorySuggestion{
		{ID: "Secrets Detection", Name: "Secrets",
			Suggestions: []*suggester.Suggestion{{Name: "Gitleaks"}}},
	}

	assert.Empty(t, testFixer(t).Snippets(suggestions, PlatformCircleCI))
}

func TestDetectPlatform(t *testing.T) {
	t.Run("github", func(t *testing.T) {
		dir := t.TempDir()
		assert.NoError(t, os.MkdirAll(filepath.Join(dir, ".github", "workflows"), 0o755))
		assert.Equal(t, PlatformGitHub, DetectPlatform(dir))
	})

	t.Run("gitlab", func(t *testing.T) {
		dir := t.TempDir()
		assert.NoError(t, os.WriteFile(filepath.Join(dir, ".gitlab-ci.yml"), []byte("x"), 0o600))
		assert.Equal(t, PlatformGitLab, DetectPlatform(dir))
	})

	t.Run("circleci", func(t *testing.T) {
		dir := t.TempDir()
		assert.NoError(t, os.MkdirAll(filepath.Join(dir, ".circleci"), 0o755))
		assert.Equal(t, PlatformCircleCI, DetectPlatform(dir))
	})

	t.Run("defaults to github", func(t *testing.T) {
		assert.Equal(t, PlatformGitHub, DetectPlatform(t.TempDir()))
	})
}

func TestShippedTemplatesLookLikeYAML(t *testing.T) {
	templates, err := readTemplates()
	assert.NoError(t, err)
	assert.NotEmpty(t, templates)

	for _, tmpl := range templates {
		assert.False(t, strings.HasPrefix(tmpl.Step, " "),
			"template for %q starts with stray indentation", tmpl.Tool)
	}
}

// A snippet that does not parse is worse than no snippet, so paste every
// shipped template into a skeleton for its platform and unmarshal it.
func TestShippedTemplatesParseWhenPasted(t *testing.T) {
	templates, err := readTemplates()
	assert.NoError(t, err)

	for _, tmpl := range templates {
		t.Run(tmpl.Tool+"/"+tmpl.Platform, func(t *testing.T) {
			var doc string
			switch tmpl.Platform {
			case PlatformGitHub:
				doc = "name: ci\non: [push]\njobs:\n  build:\n    runs-on: ubuntu-latest\n    steps:\n" +
					indent(tmpl.Step, "      ")
			default:
				doc = tmpl.Step
			}

			var parsed map[string]interface{}
			assert.NoError(t, yaml.Unmarshal([]byte(doc), &parsed),
				"template for %q does not parse:\n%s", tmpl.Tool, doc)
			assert.NotEmpty(t, parsed)
		})
	}
}

func TestGitHubTemplatesProduceRealSteps(t *testing.T) {
	templates, err := readTemplates()
	assert.NoError(t, err)

	for _, tmpl := range templates {
		if tmpl.Platform != PlatformGitHub {
			continue
		}
		t.Run(tmpl.Tool, func(t *testing.T) {
			doc := "name: ci\non: [push]\njobs:\n  build:\n    runs-on: ubuntu-latest\n    steps:\n" +
				indent(tmpl.Step, "      ")

			var wf struct {
				Jobs map[string]struct {
					Steps []map[string]interface{} `yaml:"steps"`
				} `yaml:"jobs"`
			}
			assert.NoError(t, yaml.Unmarshal([]byte(doc), &wf))
			assert.NotEmpty(t, wf.Jobs["build"].Steps, "template for %q produced no steps", tmpl.Tool)

			for _, step := range wf.Jobs["build"].Steps {
				_, hasUses := step["uses"]
				_, hasRun := step["run"]
				assert.True(t, hasUses || hasRun,
					"a step in %q has neither uses nor run: %v", tmpl.Tool, step)
			}
		})
	}
}

func indent(s, prefix string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i, l := range lines {
		if l != "" {
			lines[i] = prefix + l
		}
	}
	return strings.Join(lines, "\n") + "\n"
}

func TestRenderWritesPasteableBlocks(t *testing.T) {
	var buf bytes.Buffer
	snippets := []Snippet{
		{Category: "Secrets", Tool: "Gitleaks", Repository: "https://github.com/gitleaks/gitleaks",
			Step: "- name: Detect secrets\n  uses: gitleaks/gitleaks-action@v2"},
		{Category: "SCA", Tool: "Trivy", Repository: "https://github.com/aquasecurity/trivy",
			Step: "- name: Scan\n  uses: aquasecurity/trivy-action@master"},
	}

	assert.NoError(t, Render(&buf, snippets, PlatformGitHub))

	out := buf.String()
	assert.Contains(t, out, "Secrets is not covered. Add to .github/workflows/security.yml:")
	assert.Contains(t, out, "  - name: Detect secrets")
	assert.Contains(t, out, "https://github.com/gitleaks/gitleaks")
	assert.Contains(t, out, "SCA is not covered")
}

func TestRenderSaysSoWhenThereIsNothing(t *testing.T) {
	var buf bytes.Buffer
	assert.NoError(t, Render(&buf, nil, PlatformGitHub))
	assert.Contains(t, buf.String(), "Nothing to fix")
}

func TestRenderNamesThePlatformsFile(t *testing.T) {
	for platform, want := range map[string]string{
		PlatformGitHub:   ".github/workflows/security.yml",
		PlatformGitLab:   ".gitlab-ci.yml",
		PlatformCircleCI: ".circleci/config.yml",
	} {
		var buf bytes.Buffer
		assert.NoError(t, Render(&buf, []Snippet{{Category: "C", Step: "- run: x"}}, platform))
		assert.Contains(t, buf.String(), want)
	}
}

func TestRenderPropagatesWriteErrors(t *testing.T) {
	boom := errors.New("disk full")
	w := failWriter{err: boom}

	assert.ErrorIs(t, Render(w, []Snippet{{Category: "C", Step: "- run: x"}}, PlatformGitHub), boom)
	assert.ErrorIs(t, Render(w, nil, PlatformGitHub), boom)
}

type failWriter struct{ err error }

func (f failWriter) Write([]byte) (int, error) { return 0, f.err }

func sampleSnippets() []Snippet {
	return []Snippet{
		{Category: "Secrets", Tool: "Gitleaks", Repository: "https://github.com/gitleaks/gitleaks",
			Step: "- name: Detect secrets\n  uses: gitleaks/gitleaks-action@v2"},
		{Category: "SCA", Tool: "Trivy", Repository: "https://github.com/aquasecurity/trivy",
			Step: "- name: Scan\n  uses: aquasecurity/trivy-action@master"},
	}
}

func TestWorkflowIsAValidWorkflow(t *testing.T) {
	var wf struct {
		Name string `yaml:"name"`
		Jobs map[string]struct {
			RunsOn string                   `yaml:"runs-on"`
			Steps  []map[string]interface{} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	assert.NoError(t, yaml.Unmarshal([]byte(Workflow(sampleSnippets())), &wf))

	assert.Equal(t, "zanadir-suggested", wf.Name)
	job := wf.Jobs["suggested"]
	assert.Equal(t, "ubuntu-latest", job.RunsOn)
	assert.Len(t, job.Steps, 3, "checkout plus one step per snippet")
	for _, step := range job.Steps {
		_, hasUses := step["uses"]
		_, hasRun := step["run"]
		assert.True(t, hasUses || hasRun, "step has neither uses nor run: %v", step)
	}
}

func TestWorkflowStatesItsCost(t *testing.T) {
	out := Workflow(sampleSnippets())
	assert.Contains(t, out, generatedMarker)
	assert.Contains(t, out, "repeats checkout")
}

func TestWriteWorkflowIsIdempotent(t *testing.T) {
	dir := t.TempDir()

	path, err := WriteWorkflow(dir, sampleSnippets())
	assert.NoError(t, err)
	assert.Equal(t, generatedWorkflow, path)

	first, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(path)))
	assert.NoError(t, err)

	_, err = WriteWorkflow(dir, sampleSnippets())
	assert.NoError(t, err)
	second, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(path)))
	assert.NoError(t, err)

	assert.Equal(t, string(first), string(second))
}

func TestWriteWorkflowRefusesToClobberAHandWrittenFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, filepath.FromSlash(generatedWorkflow))
	assert.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	mine := "name: mine\non: [push]\n"
	assert.NoError(t, os.WriteFile(path, []byte(mine), 0o644))

	_, err := WriteWorkflow(dir, sampleSnippets())

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not generated by zanadir")

	after, readErr := os.ReadFile(path)
	assert.NoError(t, readErr)
	assert.Equal(t, mine, string(after), "the operator's file must be untouched")
}

func TestWriteWorkflowIsReadableByOthers(t *testing.T) {
	dir := t.TempDir()
	path, err := WriteWorkflow(dir, sampleSnippets())
	assert.NoError(t, err)

	info, err := os.Stat(filepath.Join(dir, filepath.FromSlash(path)))
	assert.NoError(t, err)
	assert.NotZero(t, info.Mode().Perm()&0o044, "a workflow is read by other tooling")
}

func TestIsGeneratedWorkflow(t *testing.T) {
	assert.True(t, IsGeneratedWorkflow(".github/workflows/zanadir-suggested.yml"))
	assert.True(t, IsGeneratedWorkflow("/repo/.github/workflows/zanadir-suggested.yml"))
	assert.False(t, IsGeneratedWorkflow(".github/workflows/ci.yml"))
}
