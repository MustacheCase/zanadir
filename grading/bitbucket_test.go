package grading

import (
	"testing"

	"github.com/MustacheCase/zanadir/parser"
	"github.com/stretchr/testify/assert"
)

// Runs a Bitbucket file through the real parser, rule catalogue and grading,
// so a mismatch between what the parser records and what a rule or grading
// reads shows up here rather than on someone's repository.
func TestBitbucketPipelinesAreGradedEndToEnd(t *testing.T) {
	verdicts := gradeFile(t, parser.NewBitbucketParser(), "bitbucket-pipelines.yml", `
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
`)

	assert.Equal(t, map[string]Verdict{
		"Unit Tests":        Unknown,
		"Secrets Detection": Partial,
		"SAST":              Enforcing,
		"SCA":               Advisory,
		"Linter":            Advisory,
		// A custom pipeline is started by hand and may be the release.
		"Supply Chain": Unknown,
	}, verdicts)
}
