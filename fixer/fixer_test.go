package fixer

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/MustacheCase/zanadir/internal/testrepo"
	"github.com/MustacheCase/zanadir/models"
	"github.com/MustacheCase/zanadir/parser"
	"github.com/MustacheCase/zanadir/rules"
	"github.com/MustacheCase/zanadir/suggester"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

	snippets := testFixer(t).Snippets(suggestions, parser.GitHub)

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

	snippets := testFixer(t).Snippets(suggestions, parser.GitHub)

	assert.Len(t, snippets, 1)
	assert.Equal(t, "Trivy", snippets[0].Tool)
}

func TestSnippetsSkipsUntemplatedCategory(t *testing.T) {
	suggestions := []*suggester.CategorySuggestion{
		{ID: "Unit Tests", Name: "Unit Tests",
			Suggestions: []*suggester.Suggestion{{Name: "JUnit"}, {Name: "pytest"}}},
	}

	assert.Empty(t, testFixer(t).Snippets(suggestions, parser.GitHub),
		"a category with no template is skipped rather than guessed at")
}

func TestSnippetsArePlatformSpecific(t *testing.T) {
	suggestions := []*suggester.CategorySuggestion{
		{ID: "Secrets Detection", Name: "Secrets",
			Suggestions: []*suggester.Suggestion{{Name: "Gitleaks"}}},
	}

	gh := testFixer(t).Snippets(suggestions, parser.GitHub)
	gl := testFixer(t).Snippets(suggestions, parser.GitLab)

	assert.Contains(t, gh[0].Step, "- name: Detect secrets")
	assert.NotContains(t, gh[0].Step, "secret_detection:")
	assert.Contains(t, gl[0].Step, "secret_detection:")
	assert.NotContains(t, gl[0].Step, "- name:")
}

func TestSnippetsEmptyForCircleCI(t *testing.T) {
	suggestions := []*suggester.CategorySuggestion{
		{ID: "Secrets Detection", Name: "Secrets",
			Suggestions: []*suggester.Suggestion{{Name: "Gitleaks"}}},
	}

	assert.Empty(t, testFixer(t).Snippets(suggestions, parser.CircleCI))
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
			case parser.GitHub:
				doc = "name: ci\non: [push]\njobs:\n  build:\n    runs-on: ubuntu-latest\n    steps:\n" +
					indent(tmpl.Step, "      ")
			case parser.Bitbucket:
				doc = bitbucketSkeleton(tmpl.Step)
			case parser.Azure:
				doc = azureSkeleton(tmpl.Step)
			case parser.Jenkins:
				// Groovy, not YAML: the round trip below is what checks it.
				return
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
		if tmpl.Platform != parser.GitHub {
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

	assert.NoError(t, Render(&buf, snippets, github))

	out := buf.String()
	assert.Contains(t, out, "Secrets is not covered. Add to .github/workflows/security.yml:")
	assert.Contains(t, out, "  - name: Detect secrets")
	assert.Contains(t, out, "https://github.com/gitleaks/gitleaks")
	assert.Contains(t, out, "SCA is not covered")
}

func TestRenderSaysSoWhenThereIsNothing(t *testing.T) {
	var buf bytes.Buffer
	assert.NoError(t, Render(&buf, nil, github))
	assert.Contains(t, buf.String(), "Nothing to fix")
}

func TestRenderNamesThePlatformsFile(t *testing.T) {
	for _, platform := range parser.Platforms {
		var buf bytes.Buffer
		assert.NoError(t, Render(&buf, []Snippet{{Category: "C", Step: "- run: x"}}, platform))
		assert.Contains(t, buf.String(), "Add to "+platform.Target+":", platform.ID)
	}
}

func TestRenderPropagatesWriteErrors(t *testing.T) {
	boom := errors.New("disk full")
	w := failWriter{err: boom}

	assert.ErrorIs(t, Render(w, []Snippet{{Category: "C", Step: "- run: x"}}, github), boom)
	assert.ErrorIs(t, Render(w, nil, github), boom)
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

func TestRenderPropagatesWriteErrorBetweenSnippets(t *testing.T) {
	boom := errors.New("disk full")
	err := Render(failWriter{err: boom}, sampleSnippets(), github)
	assert.ErrorIs(t, err, boom)
}

func TestWorkflowKeepsBlankLinesInsideAStep(t *testing.T) {
	out := Workflow([]Snippet{{
		Category: "Secrets",
		Step:     "- name: One\n  uses: a/b@v1\n\n- name: Two\n  uses: c/d@v1",
	}})

	var wf struct {
		Jobs map[string]struct {
			Steps []map[string]interface{} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	assert.NoError(t, yaml.Unmarshal([]byte(out), &wf))
	assert.Len(t, wf.Jobs["suggested"].Steps, 3, "checkout plus both steps of the snippet")
}

func TestWriteWorkflowReportsAnUnreadableTarget(t *testing.T) {
	dir := t.TempDir()
	// A directory where the workflow file belongs: readable, but not a file.
	assert.NoError(t, os.MkdirAll(filepath.Join(dir, filepath.FromSlash(generatedWorkflow)), 0o755))

	_, err := WriteWorkflow(dir, sampleSnippets())

	assert.Error(t, err)
	assert.NotContains(t, err.Error(), "not generated by zanadir",
		"a directory is a read failure, not a clobber refusal")
}

func TestWriteWorkflowReportsAnOccupiedPath(t *testing.T) {
	dir := t.TempDir()
	assert.NoError(t, os.MkdirAll(filepath.Join(dir, ".github"), 0o755))
	assert.NoError(t, os.WriteFile(filepath.Join(dir, ".github", "workflows"), []byte("x"), 0o600))

	_, err := WriteWorkflow(dir, sampleSnippets())

	assert.Error(t, err)
	assert.NotContains(t, err.Error(), "not generated by zanadir")
}

// Root ignores the permission bits these tests rely on.
func TestWriteWorkflowReportsAnUncreatableDirectory(t *testing.T) {
	testrepo.NeedsPermissions(t)
	dir := t.TempDir()
	github := filepath.Join(dir, ".github")
	assert.NoError(t, os.MkdirAll(github, 0o755))
	assert.NoError(t, os.Chmod(github, 0o500))
	t.Cleanup(func() { _ = os.Chmod(github, 0o700) })

	_, err := WriteWorkflow(dir, sampleSnippets())

	assert.ErrorIs(t, err, fs.ErrPermission)
}

// A symlinked .github/workflows would have had the workflow written wherever
// it points.
func TestWriteWorkflowStaysInTheRepository(t *testing.T) {
	dir, outside := t.TempDir(), t.TempDir()
	assert.NoError(t, os.MkdirAll(filepath.Join(dir, ".github"), 0o755))
	assert.NoError(t, os.Symlink(outside, filepath.Join(dir, ".github", "workflows")))

	_, err := WriteWorkflow(dir, sampleSnippets())

	assert.Error(t, err)
	entries, _ := os.ReadDir(outside)
	assert.Empty(t, entries)
}

func TestWriteWorkflowReportsAnUnwritableFile(t *testing.T) {
	testrepo.NeedsPermissions(t)
	dir := t.TempDir()
	path := filepath.Join(dir, filepath.FromSlash(generatedWorkflow))
	assert.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	// Generated by zanadir, so the clobber check passes, but read-only.
	assert.NoError(t, os.WriteFile(path, []byte(generatedMarker+"\nname: old\n"), 0o400))
	t.Cleanup(func() { _ = os.Chmod(path, 0o600) })

	_, err := WriteWorkflow(dir, sampleSnippets())

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to write")
	assert.NotContains(t, err.Error(), "not generated by zanadir")
}

// Trivy has a template for SCA and a different one for End Of Life. Keyed by
// tool and platform alone, the SCA filesystem scan won for both, so the
// generated end-of-life fix did no end-of-life detection at all.
func TestSnippetsPickTheTemplateForTheCategory(t *testing.T) {
	f := testFixer(t)
	trivy := []*suggester.Suggestion{{Name: "Trivy", Repository: "https://github.com/aquasecurity/trivy"}}

	eol := f.Snippets([]*suggester.CategorySuggestion{
		{ID: "End Of Life", Name: "End-of-Life Package Checkers", Suggestions: trivy},
	}, parser.GitHub)
	assert.Len(t, eol, 1)
	assert.Contains(t, eol[0].Step, "--exit-on-eol")
	assert.NotContains(t, eol[0].Step, "scan-type: fs")

	sca := f.Snippets([]*suggester.CategorySuggestion{
		{ID: "SCA", Name: "Software Composition Analysis", Suggestions: trivy},
	}, parser.GitHub)
	assert.Len(t, sca, 1)
	assert.Contains(t, sca[0].Step, "scan-type: fs")
	assert.NotContains(t, sca[0].Step, "--exit-on-eol")
}

func TestValidateRejectsUnknownCategory(t *testing.T) {
	catalogue := []suggester.CategorySuggestion{
		{ID: "End Of Life", Suggestions: []*suggester.Suggestion{{Name: "Trivy"}}},
	}
	err := validate([]Template{{Tool: "Trivy", Platform: "github", Step: "- run: x", Category: "Not A Category"}}, catalogue)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown category")
}

func bitbucketSkeleton(step string) string {
	return "pipelines:\n  pull-requests:\n    '**':\n" + indent(step, "      ")
}

func jenkinsSkeleton(stage string) string {
	return "pipeline {\n  agent any\n  stages {\n" + indent(stage, "    ") + "  }\n}\n"
}

func azureSkeleton(step string) string {
	return "pr: [main]\nsteps:\n" + indent(step, "  ")
}

// Templates are read back by the real parser, so a snippet fix prints is one a
// later scan would count.
func TestTemplatesAreReadByTheParser(t *testing.T) {
	templates, err := readTemplates()
	assert.NoError(t, err)

	// trigger is what the skeleton runs on; a Jenkinsfile records none.
	platforms := map[string]struct {
		file     string
		skeleton func(string) string
		parser   parser.Parser
		trigger  string
	}{
		parser.Bitbucket: {"bitbucket-pipelines.yml", bitbucketSkeleton, parser.NewBitbucketParser(), "pull_request"},
		parser.Azure:     {"azure-pipelines.yml", azureSkeleton, parser.NewAzureParser(), "pull_request"},
		parser.Jenkins:   {"Jenkinsfile", jenkinsSkeleton, parser.NewJenkinsParser(), ""},
	}

	for _, tmpl := range templates {
		p, ok := platforms[tmpl.Platform]
		if !ok {
			continue
		}
		t.Run(tmpl.Tool+"/"+tmpl.Platform, func(t *testing.T) {
			artifacts, err := p.parser.Parse(testrepo.Write(t, map[string]string{p.file: p.skeleton(tmpl.Step)}))
			assert.NoError(t, err)
			if assert.Len(t, artifacts, 1) && assert.Len(t, artifacts[0].Jobs, 1) {
				if p.trigger != "" {
					assert.True(t, artifacts[0].HasTrigger(p.trigger))
				}
				assert.Contains(t, strings.ToLower(artifacts[0].Jobs[0].Run+artifacts[0].Jobs[0].Package), strings.ToLower(tmpl.Tool))
			}
		})
	}
}

// github is the platform most Render tests write for.
var github, _ = parser.Lookup(parser.GitHub)

// A snippet that names an action follows a release, never a branch: a branch
// changes under the user, and zizmor reports it.
func TestTemplatesPinActionsToReleases(t *testing.T) {
	data, err := os.ReadFile("templates.yaml")
	require.NoError(t, err)
	for _, ref := range regexp.MustCompile(`uses: ([^\s@]+)@(\S+)`).FindAllStringSubmatch(string(data), -1) {
		assert.Regexp(t, `^v\d+(\.\d+)*$`, ref[2], ref[1])
	}
	assert.Contains(t, Workflow(nil), "actions/checkout@v7\n        with:\n          persist-credentials: false\n")
}

// The glsec snippet, pasted into .gitlab-ci.yml, is found by the rule that
// covers Workflow Security.
func TestGlsecTemplateIsReadByTheGitlabParser(t *testing.T) {
	templates, err := readTemplates()
	require.NoError(t, err)
	idx := slices.IndexFunc(templates, func(t Template) bool { return t.Tool == "glsec" && t.Platform == parser.GitLab })
	require.NotEqual(t, -1, idx)

	artifacts, err := parser.NewGitlabParser().Parse(testrepo.Write(t, map[string]string{".gitlab-ci.yml": templates[idx].Step}))
	require.NoError(t, err)
	require.Len(t, artifacts, 1)
	require.Len(t, artifacts[0].Jobs, 1)
	assert.Equal(t, "ghcr.io/glsec/glsec:1.23.0", artifacts[0].Jobs[0].Package)
	assert.Equal(t, "glsec .gitlab-ci.yml", artifacts[0].Jobs[0].Run)
}

// Each snippet for these tools, pasted, is a step its rule recognises.
func TestTemplatesAreReadByTheirRule(t *testing.T) {
	rs, err := rules.NewRulesService()
	require.NoError(t, err)
	templates, err := readTemplates()
	require.NoError(t, err)

	for tool, want := range map[string]struct {
		category models.CategoryTitle
		rule     string
	}{
		"poutine":             {models.WorkflowSecurity, "poutine-rule"},
		"Cisco Skill Scanner": {models.AgentSkillSecurity, "cisco-skill-scanner-rule"},
	} {
		idx := slices.IndexFunc(rs.GetCategoryRules(want.category), func(r *rules.Rule) bool { return r.ID == want.rule })
		require.NotEqual(t, -1, idx, want.rule)
		rule := rs.GetCategoryRules(want.category)[idx]

		for platform, file := range map[string]string{parser.GitHub: ".github/workflows/ci.yml", parser.GitLab: ".gitlab-ci.yml"} {
			i := slices.IndexFunc(templates, func(t Template) bool { return t.Tool == tool && t.Platform == platform })
			require.NotEqual(t, -1, i, tool, platform)
			step := templates[i].Step
			if platform == parser.GitHub {
				step = "on: pull_request\njobs:\n  a:\n    runs-on: x\n    steps:\n" + indent(step, "      ")
			}
			p, _ := parser.Lookup(platform)
			artifacts, err := p.Parser.Parse(testrepo.Write(t, map[string]string{file: step}))
			require.NoError(t, err, tool, platform)
			require.NotEmpty(t, artifacts, tool, platform)
			assert.True(t, slices.ContainsFunc(artifacts[0].Jobs, func(job *models.Job) bool { return rule.Regex.MatchString(job.Run) }), tool, platform)
		}
	}
}

// A job with no stage lands in test, which a project's own stages: list may
// not declare; GitLab then rejects the whole pipeline.
func TestGitlabTemplatesNameAStage(t *testing.T) {
	templates, err := readTemplates()
	require.NoError(t, err)
	for _, tmpl := range templates {
		if tmpl.Platform != parser.GitLab {
			continue
		}
		var jobs map[string]map[string]any
		require.NoError(t, yaml.Unmarshal([]byte(tmpl.Step), &jobs), tmpl.Tool)
		for name, job := range jobs {
			assert.Equal(t, ".pre", job["stage"], "%s job %s", tmpl.Tool, name)
		}
	}
}
