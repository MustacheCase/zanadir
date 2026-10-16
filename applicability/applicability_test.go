package applicability_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/MustacheCase/zanadir/applicability"
	"github.com/MustacheCase/zanadir/internal/testrepo"
	"github.com/stretchr/testify/assert"
)

// Ruling a category out that a repository does need is the worse mistake, so
// every marker is checked, including ones nested well below the root.
func TestIaCStaysApplicable(t *testing.T) {
	for _, file := range []string{
		"main.tf",
		"deploy/terraform/main.tf",
		"infra/vars.tfvars",
		"modules/.terraform.lock.hcl",
		"terragrunt.hcl",
		"charts/app/Chart.yaml",
		"kustomization.yml",
		"k8s/deployment.yaml",
		"Dockerfile",
		"Dockerfile.release",
		"src/app.dockerfile",
		"Containerfile",
		"docker-compose.yml",
		"compose.yaml",
		"Pulumi.yaml",
		"serverless.yml",
		"cdk.json",
		"azure/template.bicep",
		// A directory name is enough: a manifest inside one can be called
		// anything.
		"infrastructure/notes.txt",
		"helm/values-prod.txt",
	} {
		t.Run(file, func(t *testing.T) {
			assert.Equal(t, []string{"Workflow Security"},
				applicability.NotApplicable(testrepo.Touch(t, file)))
		})
	}
}

func TestIaCIsRuledOutWithoutInfrastructure(t *testing.T) {
	dir := testrepo.Touch(t, "src/main.go", "go.mod", "README.md", "docs/guide.md", ".github/workflows/ci.yml")

	assert.Equal(t, []string{"IaC Security"}, applicability.NotApplicable(dir))
}

// Every tool in the Workflow Security category reads the GitHub Actions
// format, so a repository on another platform has nothing to point them at.
func TestWorkflowSecurityNeedsGithubWorkflows(t *testing.T) {
	tests := []struct {
		name     string
		files    []string
		expected []string
	}{
		{
			name:     "github actions present",
			files:    []string{"Dockerfile", ".github/workflows/ci.yml"},
			expected: nil,
		},
		{
			name:     "a yaml workflow by its other extension",
			files:    []string{"Dockerfile", ".github/workflows/ci.yaml"},
			expected: nil,
		},
		{
			name:     "gitlab only",
			files:    []string{"Dockerfile", ".gitlab-ci.yml"},
			expected: []string{"Workflow Security"},
		},
		{
			name:     "circleci only",
			files:    []string{"Dockerfile", ".circleci/config.yml"},
			expected: []string{"Workflow Security"},
		},
		{
			// The directory exists but holds nothing GitHub would run.
			name:     "a workflows directory with no workflow in it",
			files:    []string{"Dockerfile", ".github/workflows/README.md"},
			expected: []string{"Workflow Security"},
		},
		{
			name:     "no CI at all rules out both",
			files:    []string{"go.mod"},
			expected: []string{"IaC Security", "Workflow Security"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, applicability.NotApplicable(testrepo.Touch(t, tt.files...)))
		})
	}
}

// A dependency's Dockerfile says nothing about the repository.
func TestIaCIgnoresVendoredDirectories(t *testing.T) {
	for _, vendored := range []string{
		"node_modules/pkg/Dockerfile",
		"vendor/dep/main.tf",
		"testdata/fixture/docker-compose.yml",
		"third_party/lib/Chart.yaml",
	} {
		t.Run(vendored, func(t *testing.T) {
			dir := testrepo.Touch(t, "go.mod", ".github/workflows/ci.yml", vendored)
			assert.Equal(t, []string{"IaC Security"}, applicability.NotApplicable(dir))
		})
	}
}

func TestNotApplicableWithNoDirectory(t *testing.T) {
	assert.Nil(t, applicability.NotApplicable(filepath.Join(t.TempDir(), "absent")))
}

// An empty repository has no infrastructure and no workflows either.
func TestNotApplicableWithAnEmptyRepository(t *testing.T) {
	assert.Equal(t, []string{"IaC Security", "Workflow Security"}, applicability.NotApplicable(t.TempDir()))
}

// A directory it cannot read must not crash the walk, and must not be taken as
// evidence either way.
func TestIaCToleratesAnUnreadableDirectory(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}

	dir := testrepo.Touch(t, "go.mod", ".github/workflows/ci.yml")
	locked := filepath.Join(dir, "locked")
	assert.NoError(t, os.MkdirAll(locked, 0o755))
	assert.NoError(t, os.Chmod(locked, 0o000))
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

	assert.Equal(t, []string{"IaC Security"}, applicability.NotApplicable(dir))
}

// cli/cli keeps a scripts directory inside .github/workflows, which is not a
// workflow and must not be mistaken for one.
func TestWorkflowSecurityIgnoresSubdirectories(t *testing.T) {
	dir := testrepo.Touch(t, "Dockerfile", ".github/workflows/scripts/data.yml")

	assert.Equal(t, []string{"Workflow Security"}, applicability.NotApplicable(dir))

	withWorkflow := testrepo.Touch(t, "Dockerfile", ".github/workflows/scripts/data.yml", ".github/workflows/ci.yml")
	assert.Empty(t, applicability.NotApplicable(withWorkflow))
}
