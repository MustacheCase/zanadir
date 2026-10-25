package grading

import (
	"testing"

	"github.com/MustacheCase/zanadir/parser"
	"github.com/stretchr/testify/assert"
)

// glsec in each form it takes on GitLab, through the real parser, catalogue
// and grading. GitLab records no triggers, so a glsec run that can fail the
// pipeline is unknown rather than enforcing; one that cannot is advisory.
func TestGlsecOnGitlabIsGradedEndToEnd(t *testing.T) {
	for name, tc := range map[string]struct {
		file string
		want Verdict
	}{
		"component": {
			file: "include:\n  - component: gitlab.com/glsec-io/glsec/glsec@v1.0.25\n",
			want: Unknown,
		},
		"code-quality component, which ends in || true": {
			file: "include:\n  - component: gitlab.com/glsec-io/glsec/glsec-code-quality@v1.0.25\n",
			want: Advisory,
		},
		"code-quality component written in upper case": {
			file: "include:\n  - component: gitlab.com/GLSEC-IO/glsec/glsec-code-quality@1.23.0\n",
			want: Advisory,
		},
		"image": {
			file: "glsec:\n  image:\n    name: ghcr.io/glsec/glsec:1.23.0\n    entrypoint: [\"\"]\n  script:\n    - glsec .gitlab-ci.yml\n",
			want: Unknown,
		},
		"image with allow_failure": {
			file: "glsec:\n  image: ghcr.io/glsec/glsec:1.23.0\n  allow_failure: true\n  script:\n    - glsec .gitlab-ci.yml\n",
			want: Advisory,
		},
		"downloaded binary": {
			file: "glsec:\n  script:\n    - curl -sSLO https://github.com/glsec/glsec/releases/download/v1.23.0/glsec_1.23.0_linux_amd64.tar.gz\n    - tar xzf glsec_1.23.0_linux_amd64.tar.gz\n    - ./glsec .gitlab-ci.yml || true\n",
			want: Advisory,
		},
	} {
		t.Run(name, func(t *testing.T) {
			verdicts := gradeFile(t, parser.NewGitlabParser(), ".gitlab-ci.yml", tc.file)
			assert.Equal(t, tc.want, verdicts["Workflow Security"])
		})
	}
}

// Components that only have glsec in their path don't cover the category.
func TestGlsecLookalikeComponentsDoNotCoverWorkflowSecurity(t *testing.T) {
	for _, component := range []string{
		"gitlab.com/someone/glsec/glsec@1",
		"gitlab.com/glsec-io/glsec/glsec-extra@1",
	} {
		t.Run(component, func(t *testing.T) {
			verdicts := gradeFile(t, parser.NewGitlabParser(), ".gitlab-ci.yml", "include:\n  - component: "+component+"\n")
			assert.NotContains(t, verdicts, "Workflow Security")
		})
	}
}
