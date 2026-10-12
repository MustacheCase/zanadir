package applicability_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/MustacheCase/zanadir/applicability"
	"github.com/stretchr/testify/assert"
)

func repoWith(t *testing.T, paths ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, rel := range paths {
		path := filepath.Join(dir, rel)
		assert.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		assert.NoError(t, os.WriteFile(path, []byte("x"), 0o644))
	}
	return dir
}

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
			assert.Empty(t, applicability.NotApplicable(repoWith(t, file)))
		})
	}
}

func TestIaCIsRuledOutWithoutInfrastructure(t *testing.T) {
	dir := repoWith(t, "src/main.go", "go.mod", "README.md", "docs/guide.md", ".github/workflows/ci.yml")

	assert.Equal(t, []string{"IaC Security"}, applicability.NotApplicable(dir))
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
			dir := repoWith(t, "go.mod", vendored)
			assert.Equal(t, []string{"IaC Security"}, applicability.NotApplicable(dir))
		})
	}
}

func TestNotApplicableWithNoDirectory(t *testing.T) {
	assert.Nil(t, applicability.NotApplicable(filepath.Join(t.TempDir(), "absent")))
}

// An empty repository has no infrastructure either.
func TestNotApplicableWithAnEmptyRepository(t *testing.T) {
	assert.Equal(t, []string{"IaC Security"}, applicability.NotApplicable(t.TempDir()))
}

// A directory it cannot read must not crash the walk, and must not be taken as
// evidence either way.
func TestIaCToleratesAnUnreadableDirectory(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}

	dir := repoWith(t, "go.mod")
	locked := filepath.Join(dir, "locked")
	assert.NoError(t, os.MkdirAll(locked, 0o755))
	assert.NoError(t, os.Chmod(locked, 0o000))
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

	assert.Equal(t, []string{"IaC Security"}, applicability.NotApplicable(dir))
}
