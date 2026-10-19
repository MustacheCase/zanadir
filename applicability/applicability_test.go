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
		// A manifest inside an infrastructure directory can be called anything.
		"infrastructure/network.json",
		"helm/values-prod.yaml",
		"deploy/k8s/overlays/prod/patch.yml",
		"Dockerfile-dev",
	} {
		t.Run(file, func(t *testing.T) {
			assert.Equal(t, []string{"Workflow Security"},
				applicability.NotApplicable(testrepo.Touch(t, file), nil))
		})
	}
}

func TestIaCIsRuledOutWithoutInfrastructure(t *testing.T) {
	dir := testrepo.Touch(t, "src/main.go", "go.mod", "README.md", "docs/guide.md", ".github/workflows/ci.yml")

	assert.Equal(t, []string{"IaC Security"}, applicability.NotApplicable(dir, nil))
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
			assert.Equal(t, tt.expected, applicability.NotApplicable(testrepo.Touch(t, tt.files...), nil))
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
			assert.Equal(t, []string{"IaC Security"}, applicability.NotApplicable(dir, nil))
		})
	}
}

func TestNotApplicableWithNoDirectory(t *testing.T) {
	assert.Nil(t, applicability.NotApplicable(filepath.Join(t.TempDir(), "absent"), nil))
}

// An empty repository has no infrastructure and no workflows either.
func TestNotApplicableWithAnEmptyRepository(t *testing.T) {
	assert.Equal(t, []string{"IaC Security", "Workflow Security"}, applicability.NotApplicable(t.TempDir(), nil))
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

	assert.Equal(t, []string{"IaC Security"}, applicability.NotApplicable(dir, nil))
}

// cli/cli keeps a scripts directory inside .github/workflows, which is not a
// workflow and must not be mistaken for one.
func TestWorkflowSecurityIgnoresSubdirectories(t *testing.T) {
	dir := testrepo.Touch(t, "Dockerfile", ".github/workflows/scripts/data.yml")

	assert.Equal(t, []string{"Workflow Security"}, applicability.NotApplicable(dir, nil))

	withWorkflow := testrepo.Touch(t, "Dockerfile", ".github/workflows/scripts/data.yml", ".github/workflows/ci.yml")
	assert.Empty(t, applicability.NotApplicable(withWorkflow, nil))
}

// CloudFormation, ARM templates and Kubernetes manifests are named anything,
// so they are recognised by what they hold.
func TestIaCRecognisesManifestsByContent(t *testing.T) {
	for file, content := range map[string]string{
		"template.yaml":           "AWSTemplateFormatVersion: \"2010-09-09\"\nResources: {}\n",
		"sam/app.yml":             "Transform: AWS::Serverless-2016-10-31\nResources:\n  Fn:\n    Type: AWS::Serverless::Function\n",
		"cloudformation/vpc.json": `{"Resources": {"Vpc": {"Type": "AWS::EC2::VPC"}}}`,
		"azuredeploy.json":        "{}",
		"arm/main.json":           `{"$schema": "https://schema.management.azure.com/schemas/2019-04-01/deploymentTemplate.json#"}`,
		"deploy/app.yaml":         "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: app\n",
		"ops/all.yaml":            "---\napiVersion: v1\nkind: \"Service\"\n",
	} {
		t.Run(file, func(t *testing.T) {
			dir := testrepo.Write(t, map[string]string{".github/workflows/ci.yml": "on: push\n", file: content})
			assert.Empty(t, applicability.NotApplicable(dir, nil))
		})
	}
}

// Fixtures, test data and YAML that only looks like a manifest are not
// infrastructure.
func TestIaCIgnoresTestDataAndLookalikes(t *testing.T) {
	for file, content := range map[string]string{
		"extensions/colorize-tests/test/colorize-results/Dockerfile.json": "",
		"railties/test/fixtures/Dockerfile.test":                          "",
		"tests/specs/pack/long_file_names/kubernetes/a.yaml":              "",
		"libs/core/benches/infra/x.json":                                  "",
		"spec/fixtures/main.tf":                                           "",
		"docs/Dockerfile.md":                                              "",
		"infra/README.txt":                                                "",
		"catalog-info.yaml":                                               "apiVersion: backstage.io/v1alpha1\nkind: Component\n",
		"openapi.yaml":                                                    "openapi: 3.0.0\ncomponents:\n  schemas:\n    Thing:\n      type: object\n",
		"package.json":                                                    `{"name": "app"}`,
	} {
		t.Run(file, func(t *testing.T) {
			dir := testrepo.Write(t, map[string]string{".github/workflows/ci.yml": "on: push\n", file: content})
			assert.Equal(t, []string{"IaC Security"}, applicability.NotApplicable(dir, nil))
		})
	}
}

func TestIaCDoesNotFollowASymlinkedFile(t *testing.T) {
	outside := filepath.Join(t.TempDir(), "template.yaml")
	assert.NoError(t, os.WriteFile(outside, []byte("AWSTemplateFormatVersion: x\n"), 0o600))
	dir := testrepo.Write(t, map[string]string{".github/workflows/ci.yml": "on: push\n"})
	assert.NoError(t, os.Symlink(outside, filepath.Join(dir, "template.yaml")))
	assert.Equal(t, []string{"IaC Security"}, applicability.NotApplicable(dir, nil))
}

func TestIaCToleratesAnUnreadableManifest(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores file permissions")
	}

	dir := testrepo.Write(t, map[string]string{".github/workflows/ci.yml": "on: push\n", "template.yaml": "AWSTemplateFormatVersion: x\n"})
	assert.NoError(t, os.Chmod(filepath.Join(dir, "template.yaml"), 0o000))

	assert.Equal(t, []string{"IaC Security"}, applicability.NotApplicable(dir, nil))
}

// The workflow fix --write generates is zanadir's, not the repository's.
func TestWorkflowSecurityIgnoresWhatItIsToldTo(t *testing.T) {
	dir := testrepo.Touch(t, "go.mod", ".github/workflows/zanadir-suggested.yml")
	generated := func(path string) bool { return filepath.Base(path) == "zanadir-suggested.yml" }

	assert.Equal(t, []string{"IaC Security", "Workflow Security"}, applicability.NotApplicable(dir, generated))
	assert.Equal(t, []string{"IaC Security"}, applicability.NotApplicable(dir, nil))

	dir = testrepo.Touch(t, "go.mod", ".github/workflows/zanadir-suggested.yml", ".github/workflows/ci.yml")
	assert.Equal(t, []string{"IaC Security"}, applicability.NotApplicable(dir, generated))
}
