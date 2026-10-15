package grading

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/MustacheCase/zanadir/matcher"
	"github.com/MustacheCase/zanadir/models"
	"github.com/MustacheCase/zanadir/parser"
	"github.com/MustacheCase/zanadir/rules"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Runs a Bitbucket file through the real parser, rule catalogue and grading,
// so a mismatch between what the parser records and what a rule or grading
// reads shows up here rather than on someone's repository.
func TestBitbucketPipelinesAreGradedEndToEnd(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "bitbucket-pipelines.yml"), []byte(`
pipelines:
  default:
    - step:
        script:
          - npm test
  branches:
    main:
      - step:
          script:
            - pipe: atlassian/git-secrets-scan:3.1.0
  pull-requests:
    '**':
      - step:
          script:
            - semgrep ci
            - pipe: snyk/snyk-scan:1.0.1
              variables:
                DONT_BREAK_BUILD: "true"
          after-script:
            - npx eslint .
  custom:
    sbom:
      - step:
          script:
            - syft . -o spdx-json
`), 0o600))

	artifacts, err := parser.NewBitbucketParser().Parse(dir)
	require.NoError(t, err)
	rs, err := rules.NewRulesService()
	require.NoError(t, err)

	var findings []*matcher.Finding
	for _, c := range models.CategoryTitles {
		findings = append(findings, matcher.NewMatchService().Match(artifacts, rs.GetCategoryRules(c))...)
	}
	verdicts := map[string]Verdict{}
	for category, assessment := range ByCategory(findings) {
		verdicts[category] = assessment.Verdict
	}

	assert.Equal(t, map[string]Verdict{
		"Unit Tests":        Unknown,
		"Secrets Detection": Partial,
		"SAST":              Enforcing,
		"SCA":               Advisory,
		"Linter":            Advisory,
		"Supply Chain":      Advisory,
	}, verdicts)
}
