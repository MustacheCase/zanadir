package grading

import (
	"testing"

	"github.com/MustacheCase/zanadir/internal/testrepo"
	"github.com/MustacheCase/zanadir/matcher"
	"github.com/MustacheCase/zanadir/models"
	"github.com/MustacheCase/zanadir/parser"
	"github.com/MustacheCase/zanadir/rules"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func gradeFile(t *testing.T, p parser.Parser, name, content string) map[string]Verdict {
	t.Helper()
	artifacts, err := p.Parse(testrepo.Write(t, map[string]string{name: content}))
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
	return verdicts
}

// The Azure counterpart of the Bitbucket test: real parser, real catalogue.
func TestAzurePipelinesAreGradedEndToEnd(t *testing.T) {
	verdicts := gradeFile(t, parser.NewAzureParser(), "azure-pipelines.yml", `
pr: [main]
steps:
  - task: SnykSecurityScan@1
    inputs:
      failOnIssues: false
  - task: SonarCloudAnalyze@3
  - task: VSTest@2
  - task: PublishCodeCoverageResults@2
    condition: succeededOrFailed()
  - script: gitleaks detect
    continueOnError: true
  - task: AdvancedSecurity-Codeql-Analyze@1
`)
	assert.Equal(t, map[string]Verdict{
		"SCA":               Advisory,
		"SAST":              Enforcing,
		"Unit Tests":        Enforcing,
		"Coverage":          Unknown,
		"Secrets Detection": Advisory,
	}, verdicts)
}
