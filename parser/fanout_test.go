package parser

import (
	"strings"
	"testing"

	"github.com/MustacheCase/zanadir/internal/testrepo"
	"github.com/MustacheCase/zanadir/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A file that names itself many times at each level would otherwise be read
// (entries per level)^depth times, so these tests hang without the bounds.

func parseOne(t *testing.T, p Parser, files map[string]string) *models.Artifact {
	t.Helper()
	artifacts, err := p.Parse(testrepo.Write(t, files))
	require.NoError(t, err)
	require.Len(t, artifacts, 1)
	return artifacts[0]
}

func jobRuns(artifact *models.Artifact) map[string]string {
	runs := map[string]string{}
	for _, job := range artifact.Jobs {
		runs[job.Name] = job.Run
	}
	return runs
}

func TestGitlabBoundsIncludeFanOut(t *testing.T) {
	config := "include:\n" + strings.Repeat("  - /.gitlab-ci.yml\n", 20) + "lint:\n  script: eslint .\n"
	artifact := parseOne(t, NewGitlabParser(), map[string]string{".gitlab-ci.yml": config})
	assert.Equal(t, map[string]string{"lint": "eslint ."}, jobRuns(artifact))
}

func TestGitlabStopsIncludingPastTheBudget(t *testing.T) {
	defer func(n int) { maxNestedFiles = n }(maxNestedFiles)
	maxNestedFiles = 2
	artifact := parseOne(t, NewGitlabParser(), map[string]string{
		".gitlab-ci.yml": "include:\n  - ci/*.yml\n  - other.yml\n",
		"ci/a.yml":       "a:\n  script: a\n",
		"ci/b.yml":       "b:\n  script: b\n",
		"ci/c.yml":       "c:\n  script: c\n",
		"other.yml":      "other:\n  script: other\n",
	})
	assert.Equal(t, map[string]string{"a": "a", "b": "b"}, jobRuns(artifact))
}

func TestGitlabResolvesEachExtendedJobOnce(t *testing.T) {
	var config strings.Builder
	config.WriteString(".self:\n  extends: [" + strings.Repeat(".self, ", 9) + ".self]\n  script: self\n")
	for level := range 10 {
		next := ".l" + string(rune('a'+level+1))
		config.WriteString(".l" + string(rune('a'+level)) + ":\n  extends: [" + strings.Repeat(next+", ", 9) + next + "]\n")
	}
	config.WriteString(".lk:\n  script: deep\nself:\n  extends: .self\ndeep:\n  extends: .la\n")
	artifact := parseOne(t, NewGitlabParser(), map[string]string{".gitlab-ci.yml": config.String()})
	assert.Equal(t, map[string]string{"self": "self", "deep": "deep"}, jobRuns(artifact))
}

func TestAzureBoundsTemplateFanOut(t *testing.T) {
	artifact := parseAzure(t, map[string]string{
		"azure-pipelines.yml": "pr: [main]\njobs:\n  - template: t.yml\n  - job: lint\n    steps:\n      - script: eslint .\n",
		"t.yml":               "jobs:\n" + strings.Repeat("  - template: t.yml\n", 20),
	})
	assert.Equal(t, map[string]string{"lint": "eslint ."}, jobRuns(artifact))
}

func TestAzureBoundsSubstitutionGrowth(t *testing.T) {
	artifact := parseAzure(t, map[string]string{
		"azure-pipelines.yml": "pr: [main]\nsteps:\n  - template: t.yml\n    parameters:\n      p: x\n",
		"t.yml": "parameters:\n  - name: p\n    default: ''\nsteps:\n  - template: t.yml\n    parameters:\n      p: \"" +
			strings.Repeat("${{ parameters.p }}", 30) + "\"\n  - script: echo ${{ parameters.p }}\n",
	})
	require.NotEmpty(t, artifact.Jobs)
	assert.Equal(t, "echo x", artifact.Jobs[len(artifact.Jobs)-1].Run)
}

func TestSubstituteLeavesTextAsWrittenPastTheBound(t *testing.T) {
	defer func(n int) { maxSubstituted = n }(maxSubstituted)
	maxSubstituted = 5
	text := "${{ parameters.p }} ${{ parameters.p }}"
	assert.Equal(t, text, substitute(text, parameterRef, map[string]string{"p": "abc"}))
	assert.Equal(t, "abc", substitute("${{ parameters.p }}", parameterRef, map[string]string{"p": "abc"}))
}

func TestGithubBoundsCompositeFanOut(t *testing.T) {
	artifact := parseOne(t, NewGithubParser(), map[string]string{
		".github/workflows/ci.yml": "on: pull_request\njobs:\n  scan:\n    runs-on: x\n    steps:\n      - uses: ./\n",
		"action.yml":               "runs:\n  using: composite\n  steps:\n" + strings.Repeat("    - uses: ./\n", 20) + "    - run: gitleaks detect\n      shell: bash\n",
	})
	assert.Equal(t, "gitleaks detect", artifact.Jobs[len(artifact.Jobs)-2].Run)
	assert.LessOrEqual(t, len(artifact.Jobs), 21*maxNestedFiles+1)
}

func TestGithubBoundsSubstitutionGrowth(t *testing.T) {
	artifact := parseOne(t, NewGithubParser(), map[string]string{
		".github/workflows/ci.yml": "on: pull_request\njobs:\n  scan:\n    runs-on: x\n    steps:\n      - uses: ./\n        with:\n          i: x\n",
		"action.yml": "inputs:\n  i:\n    default: ''\nruns:\n  using: composite\n  steps:\n    - uses: ./\n      with:\n        i: \"" +
			strings.Repeat("${{ inputs.i }}", 30) + "\"\n    - run: echo ${{ inputs.i }}\n      shell: bash\n",
	})
	require.NotEmpty(t, artifact.Jobs)
	assert.Equal(t, "echo x", artifact.Jobs[len(artifact.Jobs)-2].Run)
}
